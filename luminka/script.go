//go:build scripts

// FILE: luminka/script.go
// PURPOSE: Execute validated files through the constrained script capability lane.
// OWNS: Script runner invocation, timeout handling, and output capture.
// EXPORTS: ScriptBridge, NewScriptBridge, scriptSupportAvailable
// DOCS: docs/spec.md, docs/arch.md

package luminka

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ScriptBridge struct {
	root           string
	defaultTimeout time.Duration
	scriptAssets   fs.FS
}

func NewScriptBridge(root string, defaultTimeout time.Duration) *ScriptBridge {
	return &ScriptBridge{root: root, defaultTimeout: defaultTimeout}
}

func scriptSupportAvailable() bool {
	return true
}

func (sb *ScriptBridge) Exec(runner string, file string, args []string, timeout time.Duration) (stdout string, stderr string, code int, err error) {
	if sb == nil {
		return "", "", -1, errors.New("script bridge is required")
	}
	if runner == "" {
		return "", "", -1, errors.New("runner is required")
	}
	if file == "" {
		return "", "", -1, errors.New("file is required")
	}

	resolvedFile, cleanup, err := sb.resolveScriptFile(file)
	if err != nil {
		return "", "", -1, err
	}
	defer cleanup()

	info, err := os.Stat(resolvedFile)
	if err != nil {
		return "", "", -1, err
	}
	if info.IsDir() {
		return "", "", -1, errors.New("file is required")
	}

	resolvedTimeout := timeout
	if resolvedTimeout <= 0 {
		resolvedTimeout = sb.defaultTimeout
	}
	ctx := context.Background()
	var cancel context.CancelFunc = func() {}
	if resolvedTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, resolvedTimeout)
	}
	defer cancel()

	cmdArgs := append([]string{resolvedFile}, args...)
	cmd := exec.CommandContext(ctx, runner, cmdArgs...)
	cmd.Dir = sb.root

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err = cmd.Run()
	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()
	if err == nil {
		return stdout, stderr, 0, nil
	}
	code = -1
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	}
	return stdout, stderr, code, err
}

func (sb *ScriptBridge) ExecStream(rt *Runtime, conn *wsConnection, id json.RawMessage, runner string, file string, args []string, timeout time.Duration) error {
	if sb == nil {
		return errors.New("script bridge is required")
	}
	if rt == nil {
		return errors.New("runtime is required")
	}
	if conn == nil {
		return errors.New("websocket connection is required")
	}
	if runner == "" {
		return errors.New("runner is required")
	}
	if file == "" {
		return errors.New("file is required")
	}

	resolvedFile, cleanup, err := sb.resolveScriptFile(file)
	if err != nil {
		return err
	}
	defer cleanup()

	info, err := os.Stat(resolvedFile)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("file is required")
	}

	resolvedTimeout := timeout
	if resolvedTimeout <= 0 {
		resolvedTimeout = sb.defaultTimeout
	}
	ctx := context.Background()
	var cancel context.CancelFunc = func() {}
	if resolvedTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, resolvedTimeout)
	}
	defer cancel()

	cmdArgs := append([]string{resolvedFile}, args...)
	cmd := exec.CommandContext(ctx, runner, cmdArgs...)
	cmd.Dir = sb.root

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	if rt.streams == nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return errors.New("stream registry is unavailable")
	}
	stream := rt.streams.registerProcessOutput(conn)
	if stream == nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return errors.New("stream registry is unavailable")
	}
	defer rt.streams.remove(stream.id)

	writer := newExecStreamWriter(conn, stream.id)
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(2)
	go pumpReaderToStream(stdoutPipe, "stdout", writer, errCh, &wg)
	go pumpReaderToStream(stderrPipe, "stderr", writer, errCh, &wg)

	waitErr := cmd.Wait()
	wg.Wait()

	streamErr := firstStreamError(errCh)
	ok := waitErr == nil && streamErr == nil
	code := 0
	errText := ""
	if streamErr != nil {
		code = -1
		errText = streamErr.Error()
	}
	if waitErr != nil {
		ok = false
		code = commandExitCode(waitErr)
		if errText == "" {
			errText = waitErr.Error()
		}
	}
	if err := writeWSMessage(conn, wsMessage{Event: "script_response", ID: id, Ok: boolPtr(ok), Code: intPtr(code), Error: errText, StreamID: stream.id}); err != nil {
		return err
	}
	return nil
}

func (sb *ScriptBridge) resolveScriptFile(file string) (string, func(), error) {
	if strings.HasPrefix(file, "internal:") {
		return sb.materializeInternalScript(strings.TrimPrefix(file, "internal:"))
	}
	resolvedFile, err := NewFSBridge(sb.root).sanitize(file)
	if err != nil {
		return "", nil, err
	}
	return resolvedFile, func() {}, nil
}

func (sb *ScriptBridge) materializeInternalScript(selector string) (string, func(), error) {
	if selector == "" {
		return "", nil, errors.New("invalid internal selector: path is required")
	}
	if sb.scriptAssets == nil {
		return "", nil, errors.New("embedded script bundle is required")
	}
	resolved, err := resolveEmbeddedPath(selector)
	if err != nil {
		return "", nil, err
	}
	data, err := fs.ReadFile(sb.scriptAssets, resolved)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil, errors.New("embedded script not found: " + resolved)
		}
		return "", nil, err
	}
	tempFile, err := os.CreateTemp("", "luminka-script-*-"+path.Base(resolved))
	if err != nil {
		return "", nil, err
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		return "", nil, err
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempFile.Name())
		return "", nil, err
	}
	return tempFile.Name(), func() { _ = os.Remove(tempFile.Name()) }, nil
}

func resolveEmbeddedPath(selector string) (string, error) {
	cleaned := path.Clean(filepath.ToSlash(selector))
	if cleaned == "." {
		return "", errors.New("invalid internal selector: path is required")
	}
	if !fs.ValidPath(cleaned) {
		return "", errors.New("invalid internal selector: path escapes embedded bundle")
	}
	return cleaned, nil
}
