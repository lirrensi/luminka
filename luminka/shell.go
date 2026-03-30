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
	"errors"
	"os/exec"
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
