//go:build !windows

// FILE: luminka/focus_other.go
// PURPOSE: Provide best-effort no-op foregrounding on non-Windows platforms.
// OWNS: Non-Windows duplicate webview focus behavior.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_webview_focus_broadcast_2026-04-27.md

package luminka

func focusExistingInstance(record instanceRecord) error {
	return validateFocusRecord(record)
}
