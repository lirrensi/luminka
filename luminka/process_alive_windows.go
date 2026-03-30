//go:build windows

// FILE: luminka/process_alive_windows.go
// PURPOSE: Check whether a lock-file PID is still running on Windows.
// OWNS: Windows process-liveness probing for lock recovery.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_phase1_runtime_2026-03-30.md

package luminka

import "syscall"

const (
	windowsProcessQueryLimitedInformation = 0x1000
	windowsStillActiveExitCode            = 259
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(windowsProcessQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)

	var exitCode uint32
	if err := syscall.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == windowsStillActiveExitCode
}
