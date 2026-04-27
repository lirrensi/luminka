//go:build !windows

// FILE: luminka/window_identity_other.go
// PURPOSE: Provide no-op webview window identity detection on non-Windows platforms.
// OWNS: Non-Windows window identity detection behavior.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_webview_focus_broadcast_2026-04-27.md

package luminka

func detectCurrentWindowIdentity(title string) instanceWindowRecord {
	return instanceWindowRecord{}
}
