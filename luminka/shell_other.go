//go:build !shell

// FILE: luminka/shell_other.go
// PURPOSE: Report that the unrestricted shell lane is unavailable in non-shell builds.
// OWNS: Shell bridge stubbing and capability availability reporting.
// EXPORTS: ShellBridge, NewShellBridge, shellSupportAvailable
// DOCS: docs/spec.md, docs/arch.md

package luminka

import (
	"errors"
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
	return false
}

func (sb *ShellBridge) Exec(cmd string, args []string, timeout time.Duration) (stdout string, stderr string, code int, err error) {
	return "", "", -1, errors.New("shell support is not available in this build; rebuild with -tags shell")
}
