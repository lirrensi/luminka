//go:build linux

// FILE: luminka/fdatasync_linux.go
// PURPOSE: Provide fdatasync for Linux (data-only sync, not metadata).
// OWNS: fdatasync implementation for Linux.
// EXPORTS: fdatasync
// DOCS: agent_chat/plan_critical_fixes_2026-05-25.md

package luminka

import (
	"os"
	"syscall"
)

func fdatasync(file *os.File) error {
	return syscall.Fdatasync(int(file.Fd()))
}
