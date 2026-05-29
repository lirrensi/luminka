// FILE: luminka/ws_broadcast.go
// PURPOSE: Route transient local WebSocket broadcast messages between active clients.
// OWNS: Broadcast request validation, target selection, push delivery, and acknowledgements.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_webview_focus_broadcast_2026-04-27.md

package luminka

func (rt *Runtime) handleBroadcastRequest(sender *WSConnection, request WSMessage, payload []byte) error {
	if rt == nil {
		return WriteErrorResponse(sender, request.ID, "runtime is required")
	}
	if request.Channel == "" {
		return WriteWSMessage(sender, WSMessage{Event: "response:ws:broadcast", ID: request.ID, Ok: boolPtr(false), Error: "broadcast channel is required"})
	}
	targets := rt.connectionSnapshot()
	if !request.Echo {
		targets = rt.connectionSnapshotExcluding(sender)
	}
	message := WSMessage{Event: "ws:broadcast", Channel: request.Channel, Data: request.Data, ContentType: request.ContentType}
	// Iterate targets first and collect delivery errors.
	var firstErr error
	for _, target := range targets {
		if err := WriteWSFrame(target, message, payload); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// Then send response with appropriate ok status.
	ok := firstErr == nil
	return WriteWSMessage(sender, WSMessage{Event: "response:ws:broadcast", ID: request.ID, Ok: boolPtr(ok)})
}
