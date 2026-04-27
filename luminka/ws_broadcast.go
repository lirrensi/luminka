// FILE: luminka/ws_broadcast.go
// PURPOSE: Route transient local WebSocket broadcast messages between active clients.
// OWNS: Broadcast request validation, target selection, push delivery, and acknowledgements.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_webview_focus_broadcast_2026-04-27.md

package luminka

import "fmt"

func (rt *Runtime) handleBroadcastRequest(sender *wsConnection, request wsMessage, payload []byte) error {
	if rt == nil {
		return writeErrorResponse(sender, request.ID, "runtime is required")
	}
	if request.Channel == "" {
		return writeWSMessage(sender, wsMessage{Event: "broadcast_response", ID: request.ID, Ok: boolPtr(false), Error: "broadcast channel is required"})
	}
	targets := rt.connectionSnapshot()
	if !request.Echo {
		targets = rt.connectionSnapshotExcluding(sender)
	}
	message := wsMessage{Event: "broadcast", Channel: request.Channel, Data: request.Data, ContentType: request.ContentType}
	var firstErr error
	for _, target := range targets {
		if err := writeWSFrame(target, message, payload); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := writeWSMessage(sender, wsMessage{Event: "broadcast_response", ID: request.ID, Ok: boolPtr(true)}); err != nil {
		return err
	}
	if firstErr != nil {
		return fmt.Errorf("broadcast delivery failed: %w", firstErr)
	}
	return nil
}
