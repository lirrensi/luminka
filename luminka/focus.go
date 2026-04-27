// FILE: luminka/focus.go
// PURPOSE: Share validation for best-effort existing-instance foregrounding.
// OWNS: Common focus input validation used by platform adapters.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_webview_focus_broadcast_2026-04-27.md

package luminka

import "fmt"

func validateFocusRecord(record instanceRecord) error {
	if record.PID <= 0 {
		return fmt.Errorf("invalid instance pid %d", record.PID)
	}
	return nil
}
