//go:build windows

// FILE: luminka/focus_windows.go
// PURPOSE: Foreground an existing Luminka webview window using Win32 APIs.
// OWNS: Windows duplicate webview focus behavior and visible-window enumeration.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_webview_focus_broadcast_2026-04-27.md

package luminka

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const swRestore = 9

var (
	user32                    = syscall.NewLazyDLL("user32.dll")
	procIsWindow              = user32.NewProc("IsWindow")
	procShowWindow            = user32.NewProc("ShowWindow")
	procSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop      = user32.NewProc("BringWindowToTop")
	procEnumWindows           = user32.NewProc("EnumWindows")
	procGetWindowThreadProcID = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible       = user32.NewProc("IsWindowVisible")
)

func focusExistingInstance(record instanceRecord) error {
	if err := validateFocusRecord(record); err != nil {
		return err
	}
	if hwnd, ok := storedWindowsHandle(record); ok && focusWindowHandle(hwnd) == nil {
		return nil
	}
	for _, hwnd := range enumVisibleWindowsForPID(record.PID) {
		if focusWindowHandle(hwnd) == nil {
			return nil
		}
	}
	return fmt.Errorf("no visible window found for pid %d", record.PID)
}

func storedWindowsHandle(record instanceRecord) (uintptr, bool) {
	if record.Window.Platform != "windows" || record.Window.ID == "" {
		return 0, false
	}
	raw := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(record.Window.ID)), "0x")
	hwnd, err := strconv.ParseUint(raw, 16, 0)
	if err != nil {
		return 0, false
	}
	return uintptr(hwnd), hwnd != 0
}

func focusWindowHandle(hwnd uintptr) error {
	if hwnd == 0 || !isWindow(hwnd) {
		return fmt.Errorf("invalid window handle")
	}
	if _, _, err := procShowWindow.Call(hwnd, swRestore); err != syscall.Errno(0) {
		return err
	}
	if _, _, err := procBringWindowToTop.Call(hwnd); err != syscall.Errno(0) {
		return err
	}
	if _, _, err := procSetForegroundWindow.Call(hwnd); err != syscall.Errno(0) {
		return err
	}
	return nil
}

func isWindow(hwnd uintptr) bool {
	ret, _, _ := procIsWindow.Call(hwnd)
	return ret != 0
}

func enumVisibleWindowsForPID(pid int) []uintptr {
	var handles []uintptr
	callback := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		if windowVisibleForPID(hwnd, pid) {
			handles = append(handles, hwnd)
		}
		return 1
	})
	procEnumWindows.Call(callback, uintptr(unsafe.Pointer(&handles)))
	return handles
}

func windowVisibleForPID(hwnd uintptr, pid int) bool {
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	if visible == 0 {
		return false
	}
	var windowPID uint32
	procGetWindowThreadProcID.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))
	return int(windowPID) == pid
}
