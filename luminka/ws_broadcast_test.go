// FILE: luminka/ws_broadcast_test.go
// PURPOSE: Verify transient WebSocket broadcast routing between active runtime clients.
// OWNS: Broadcast acknowledgements, delivery, echo behavior, and validation tests.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_webview_focus_broadcast_2026-04-27.md

package luminka

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestBroadcastDeliversToOtherConnectionAndAcknowledgesSender(t *testing.T) {
	rt, sender, receiver := newBroadcastRuntime(t)
	payload := []byte{1, 2, 3}
	err := rt.handleBroadcastRequest(sender, wsMessage{Event: "broadcast", ID: json.RawMessage(`"b1"`), Channel: "workspace", Data: json.RawMessage(`{"type":"ping"}`), ContentType: "application/json"}, payload)
	if err != nil {
		t.Fatalf("handleBroadcastRequest() error = %v", err)
	}

	delivered, deliveredPayload := mustReadFakeWSFrame(t, receiver.conn.(*fakeWebSocketConn))
	if delivered["event"] != "broadcast" || delivered["channel"] != "workspace" {
		t.Fatalf("broadcast frame = %#v, want workspace broadcast", delivered)
	}
	data, ok := delivered["data"].(map[string]any)
	if !ok || data["type"] != "ping" {
		t.Fatalf("broadcast data = %#v, want ping object", delivered["data"])
	}
	if string(deliveredPayload) != string(payload) {
		t.Fatalf("payload = %v, want %v", deliveredPayload, payload)
	}
	assertWSOK(t, mustReadFakeWSMessage(t, sender.conn.(*fakeWebSocketConn)), "broadcast_response", "b1")
}

func TestBroadcastDoesNotEchoToSenderByDefault(t *testing.T) {
	rt, sender, _ := newBroadcastRuntime(t)
	if err := rt.handleBroadcastRequest(sender, wsMessage{Event: "broadcast", ID: json.RawMessage(`"b2"`), Channel: "workspace"}, nil); err != nil {
		t.Fatalf("handleBroadcastRequest() error = %v", err)
	}
	assertWSOK(t, mustReadFakeWSMessage(t, sender.conn.(*fakeWebSocketConn)), "broadcast_response", "b2")
	assertNoFakeWSMessage(t, sender.conn.(*fakeWebSocketConn))
}

func TestBroadcastEchoDeliversToSender(t *testing.T) {
	rt, sender, _ := newBroadcastRuntime(t)
	if err := rt.handleBroadcastRequest(sender, wsMessage{Event: "broadcast", ID: json.RawMessage(`"b3"`), Channel: "workspace", Echo: true}, nil); err != nil {
		t.Fatalf("handleBroadcastRequest() error = %v", err)
	}
	msg := mustReadFakeWSMessage(t, sender.conn.(*fakeWebSocketConn))
	if msg["event"] != "broadcast" || msg["channel"] != "workspace" {
		t.Fatalf("echo frame = %#v, want workspace broadcast", msg)
	}
	assertWSOK(t, mustReadFakeWSMessage(t, sender.conn.(*fakeWebSocketConn)), "broadcast_response", "b3")
}

func TestBroadcastRejectsEmptyChannel(t *testing.T) {
	rt, sender, _ := newBroadcastRuntime(t)
	if err := rt.handleBroadcastRequest(sender, wsMessage{Event: "broadcast", ID: json.RawMessage(`"b4"`)}, nil); err != nil {
		t.Fatalf("handleBroadcastRequest() error = %v", err)
	}
	assertWSFailure(t, mustReadFakeWSMessage(t, sender.conn.(*fakeWebSocketConn)), "broadcast_response", "b4", "broadcast channel is required")
}

func newBroadcastRuntime(t *testing.T) (*Runtime, *wsConnection, *wsConnection) {
	t.Helper()
	rt := &Runtime{connections: make(map[*wsConnection]struct{})}
	sender := rt.registerConnection(newFakeWebSocketConn())
	receiver := rt.registerConnection(newFakeWebSocketConn())
	t.Cleanup(func() {
		rt.unregisterConnection(sender)
		rt.unregisterConnection(receiver)
	})
	return rt, sender, receiver
}

func mustReadFakeWSMessage(t *testing.T, conn *fakeWebSocketConn) map[string]any {
	t.Helper()
	msg, _ := mustReadFakeWSFrame(t, conn)
	return msg
}

func mustReadFakeWSFrame(t *testing.T, conn *fakeWebSocketConn) (map[string]any, []byte) {
	t.Helper()
	select {
	case frame := <-conn.writes:
		if frame.msgType != websocket.BinaryMessage {
			t.Fatalf("message type = %d, want binary", frame.msgType)
		}
		var msg map[string]any
		payload, err := decodeFrame(frame.data, &msg)
		if err != nil {
			t.Fatalf("decodeFrame() error = %v", err)
		}
		return msg, payload
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket frame")
	}
	return nil, nil
}

func assertNoFakeWSMessage(t *testing.T, conn *fakeWebSocketConn) {
	t.Helper()
	select {
	case frame := <-conn.writes:
		t.Fatalf("unexpected websocket frame %#v", frame)
	case <-time.After(50 * time.Millisecond):
	}
}
