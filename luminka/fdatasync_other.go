//go:build !linux

// FILE: luminka/fdatasync_other.go
// PURPOSE: Provide fdatasync fallback (Sync) for non-Linux platforms.
// OWNS: fdatasync fallback implementation.
// EXPORTS: fdatasync
// DOCS: agent_chat/plan_critical_fixes_2026-05-25.md

package luminka

import "os"

func fdatasync(file *os.File) error {
	return file.Sync()
}
