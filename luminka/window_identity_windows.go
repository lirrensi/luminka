//go:build windows

// FILE: luminka/window_identity_windows.go
// PURPOSE: Detect the current Windows webview top-level window handle.
// OWNS: Windows window identity discovery for future duplicate-launch focus.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_webview_focus_broadcast_2026-04-27.md

package luminka

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var procGetWindowTextW = user32.NewProc("GetWindowTextW")

func detectCurrentWindowIdentity(title string) instanceWindowRecord {
	hwnd := selectCurrentProcessWindow(os.Getpid(), title)
	if hwnd == 0 {
		return instanceWindowRecord{}
	}
	return instanceWindowRecord{Platform: "windows", ID: fmt.Sprintf("0x%X", hwnd)}
}

func selectCurrentProcessWindow(pid int, title string) uintptr {
	for _, hwnd := range enumVisibleWindowsForPID(pid) {
		if title == "" || windowTitle(hwnd) == title {
			return hwnd
		}
	}
	return 0
}

func windowTitle(hwnd uintptr) string {
	buf := make([]uint16, 256)
	ret, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if ret == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:ret])
}
