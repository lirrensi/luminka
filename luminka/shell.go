//go:build shell

// FILE: luminka/shell.go
// PURPOSE: Execute unrestricted local commands for the shell capability lane.
// OWNS: Shell command invocation, timeout handling, and output capture.
// EXPORTS: ShellBridge, NewShellBridge, shellSupportAvailable
// DOCS: docs/spec.md, docs/arch.md

package luminka

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"sync"
	"time"
)

type ShellBridge struct {
	root           string
	defaultTimeout time.Duration
}

func NewShellBridge(root string, defaultTimeout time.Duration) *ShellBridge {
	return &ShellBridge{root: root, defaultTimeout: defaultTimeout}
}

func shellSupportAvailable() bool {
	return true
}

func (sb *ShellBridge) Exec(cmd string, args []string, timeout time.Duration) (stdout string, stderr string, code int, err error) {
	if sb == nil {
		return "", "", -1, errors.New("shell bridge is required")
	}
	if cmd == "" {
		return "", "", -1, errors.New("cmd is required")
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

	command := exec.CommandContext(ctx, cmd, args...)
	command.Dir = sb.root

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	command.Stdout = &stdoutBuf
	command.Stderr = &stderrBuf

	err = command.Run()
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

func (sb *ShellBridge) ExecStream(rt *Runtime, conn *wsConnection, id json.RawMessage, cmd string, args []string, timeout time.Duration) error {
	if sb == nil {
		return errors.New("shell bridge is required")
	}
	if rt == nil {
		return errors.New("runtime is required")
	}
	if conn == nil {
		return errors.New("websocket connection is required")
	}
	if cmd == "" {
		return errors.New("cmd is required")
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

	command := exec.CommandContext(ctx, cmd, args...)
	command.Dir = sb.root

	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}

	if rt.streams == nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return errors.New("stream registry is unavailable")
	}
	stream := rt.streams.registerProcessOutput(conn)
	if stream == nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return errors.New("stream registry is unavailable")
	}
	defer rt.streams.remove(stream.id)

	writer := newExecStreamWriter(conn, stream.id)
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(2)
	go pumpReaderToStream(stdoutPipe, "stdout", writer, errCh, &wg)
	go pumpReaderToStream(stderrPipe, "stderr", writer, errCh, &wg)

	waitErr := command.Wait()
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
	if err := writeWSMessage(conn, wsMessage{Event: "shell_response", ID: id, Ok: boolPtr(ok), Code: intPtr(code), Error: errText, StreamID: stream.id}); err != nil {
		return err
	}
	return nil
}
