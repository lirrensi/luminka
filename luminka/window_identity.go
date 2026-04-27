// FILE: luminka/window_identity.go
// PURPOSE: Persist best-effort platform window identity for webview relaunch focus.
// OWNS: Runtime lock record updates for detected webview window identity.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_webview_focus_broadcast_2026-04-27.md

package luminka

func writeRuntimeWindowIdentity(rt *Runtime, window instanceWindowRecord) error {
	if rt == nil || rt.LockPath == "" || window.Platform == "" || window.ID == "" {
		return nil
	}
	record := instanceRecord{PID: rt.PID, Port: rt.Port, Mode: rt.Mode, Window: window}
	if existing, err := readLockRecord(rt.LockPath); err == nil && existing != nil {
		record = *existing
		record.PID = rt.PID
		record.Port = rt.Port
		record.Mode = rt.Mode
		record.Window = window
	}
	return writeInstanceRecord(rt.LockPath, record)
}
