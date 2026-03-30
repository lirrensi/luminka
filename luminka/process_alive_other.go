//go:build !windows

// FILE: luminka/process_alive_other.go
// PURPOSE: Check whether a lock-file PID is still running on non-Windows platforms.
// OWNS: Cross-platform process-liveness probing for lock recovery.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_phase1_runtime_2026-03-30.md

package luminka

import (
	"os"
	"syscall"
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
