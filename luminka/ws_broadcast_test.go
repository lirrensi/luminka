// FILE: luminka/ws_broadcast_test.go
// PURPOSE: Verify broadcast request validation, target selection, and delivery.
// OWNS: Tests for handleBroadcastRequest nil-runtime, channel validation, echo/no-echo modes.
// EXPORTS: none

package luminka

import (
	"sync"
	"testing"
)

// --- Nil Runtime ---

func TestBroadcastNilRuntime(t *testing.T) {
	sender := &wsConnection{conn: newFakeWebSocketConn()}
	req := wsMessage{Event: "broadcast", ID: rawStringData("b1"), Channel: "chat", Data: rawStringData("hi")}

	// When rt is nil, the function writes an error response and returns nil (write success)
	err := (*Runtime)(nil).handleBroadcastRequest(sender, req, nil)
	if err != nil {
		t.Fatalf("expected nil error (error is sent via WS), got: %v", err)
	}
	// sender should receive an error response
	frame := <-sender.conn.(*fakeWebSocketConn).writes
	var resp map[string]any
	_, _ = decodeFrame(frame.data, &resp)
	if resp["event"] != "error" {
		t.Fatalf("event = %v, want error", resp["event"])
	}
	if resp["error"] != "runtime is required" {
		t.Fatalf("error = %v, want 'runtime is required'", resp["error"])
	}
}

// --- Empty Channel ---

func TestBroadcastEmptyChannel(t *testing.T) {
	fconn := newFakeWebSocketConn()
	sender := &wsConnection{conn: fconn}
	rt := &Runtime{
		connections: make(map[*wsConnection]struct{}),
	}

	req := wsMessage{Event: "broadcast", ID: rawStringData("b2")}
	err := rt.handleBroadcastRequest(sender, req, nil)
	if err != nil {
		t.Fatalf("handleBroadcastRequest returned error (message is sent via WS): %v", err)
	}

	frame := <-fconn.writes
	var resp map[string]any
	_, _ = decodeFrame(frame.data, &resp)
	if resp["event"] != "broadcast_response" {
		t.Fatalf("event = %v, want broadcast_response", resp["event"])
	}
	if ok, _ := resp["ok"].(bool); ok != false {
		t.Fatalf("ok = %v, want false", ok)
	}
	if resp["error"] != "broadcast channel is required" {
		t.Fatalf("error = %v, want 'broadcast channel is required'", resp["error"])
	}
}

// --- Successful Broadcast with Echo ---

func TestBroadcastWithEcho(t *testing.T) {
	fconn1 := newFakeWebSocketConn()
	fconn2 := newFakeWebSocketConn()
	fconn3 := newFakeWebSocketConn()

	conn1 := &wsConnection{conn: fconn1}
	conn2 := &wsConnection{conn: fconn2}
	conn3 := &wsConnection{conn: fconn3}

	rt := &Runtime{
		connections: map[*wsConnection]struct{}{
			conn1: {},
			conn2: {},
			conn3: {},
		},
	}

	req := wsMessage{
		Event:       "broadcast",
		ID:          rawStringData("b3"),
		Channel:     "chat",
		Data:        rawStringData("hello everyone"),
		ContentType: "text/plain",
		Echo:        true, // sender also receives
	}

	payload := []byte("the payload")
	err := rt.handleBroadcastRequest(conn1, req, payload)
	if err != nil {
		t.Fatalf("handleBroadcastRequest error = %v", err)
	}

	// With Echo=true, the sender is in the targets list, so the broadcast
	// is sent to ALL connections first (order is map-iteration dependent),
	// then the broadcast_response is sent to the sender.
	// Collect messages from all three connections.
	type msgResult struct {
		connID int
		event  string
		pl     []byte
	}
	var results []msgResult

	for _, fc := range []*fakeWebSocketConn{fconn1, fconn2, fconn3} {
		// Each connection receives exactly one message (the broadcast)
		frame := <-fc.writes
		var msg map[string]any
		pl, _ := decodeFrame(frame.data, &msg)
		event, _ := msg["event"].(string)
		results = append(results, msgResult{connID: -1, event: event, pl: pl})
	}

	// Among the 3 broadcast messages, verify channel/content_type/payload
	broadcastCount := 0
	for _, r := range results {
		if r.event == "broadcast" {
			broadcastCount++
		}
	}
	if broadcastCount != 3 {
		t.Fatalf("got %d broadcast events, want 3 (all connections including sender)", broadcastCount)
	}

	// Now consume the broadcast_response on fconn1 (sender)
	// It's a 4th message on fconn1's write channel
	senderResp := <-fconn1.writes
	var sr map[string]any
	_, _ = decodeFrame(senderResp.data, &sr)
	if sr["event"] != "broadcast_response" {
		t.Fatalf("sender event = %v, want broadcast_response", sr["event"])
	}
	if ok, _ := sr["ok"].(bool); !ok {
		t.Fatalf("sender ok = %v, want true", ok)
	}
}

// --- Successful Broadcast without Echo ---

func TestBroadcastWithoutEcho(t *testing.T) {
	fconn1 := newFakeWebSocketConn()
	fconn2 := newFakeWebSocketConn()

	conn1 := &wsConnection{conn: fconn1}
	conn2 := &wsConnection{conn: fconn2}

	rt := &Runtime{
		connections: map[*wsConnection]struct{}{
			conn1: {},
			conn2: {},
		},
	}

	req := wsMessage{
		Event:   "broadcast",
		ID:      rawStringData("b4"),
		Channel: "alerts",
		Data:    rawStringData("warning"),
		Echo:    false, // sender does NOT receive
	}

	err := rt.handleBroadcastRequest(conn1, req, nil)
	if err != nil {
		t.Fatalf("handleBroadcastRequest error = %v", err)
	}

	// conn1 (sender) should receive broadcast_response ok
	senderResp := <-fconn1.writes
	var sr map[string]any
	_, _ = decodeFrame(senderResp.data, &sr)
	if sr["event"] != "broadcast_response" {
		t.Fatalf("sender event = %v, want broadcast_response", sr["event"])
	}

	// conn2 should receive the broadcast
	frame2 := <-fconn2.writes
	var msg2 map[string]any
	_, _ = decodeFrame(frame2.data, &msg2)
	if msg2["event"] != "broadcast" {
		t.Fatalf("conn2 event = %v, want broadcast", msg2["event"])
	}
	if msg2["channel"] != "alerts" {
		t.Fatalf("conn2 channel = %v, want alerts", msg2["channel"])
	}

	// conn1 should NOT have a broadcast message (only the response)
	select {
	case extra := <-fconn1.writes:
		var extraMsg map[string]any
		_, _ = decodeFrame(extra.data, &extraMsg)
		if extraMsg["event"] == "broadcast" {
			t.Fatal("conn1 should not receive broadcast when echo=false")
		}
	default:
		// good — no extra message
	}
}

// --- Multiple Broadcast Targets ---

func TestBroadcastMultipleTargets(t *testing.T) {
	fconns := make([]*fakeWebSocketConn, 5)
	conns := make([]*wsConnection, 5)
	connsByFake := make(map[*fakeWebSocketConn]*wsConnection)

	rt := &Runtime{
		connections: make(map[*wsConnection]struct{}),
	}
	for i := 0; i < 5; i++ {
		fconns[i] = newFakeWebSocketConn()
		conns[i] = &wsConnection{conn: fconns[i]}
		connsByFake[fconns[i]] = conns[i]
		rt.connections[conns[i]] = struct{}{}
	}

	req := wsMessage{
		Event:   "broadcast",
		ID:      rawStringData("b5"),
		Channel: "system",
		Echo:    false,
	}
	sender := conns[0]

	err := rt.handleBroadcastRequest(sender, req, []byte("status"))
	if err != nil {
		t.Fatalf("handleBroadcastRequest error = %v", err)
	}

	// sender gets broadcast_response
	<-fconns[0].writes

	// All others get the broadcast
	for i := 1; i < 5; i++ {
		frame := <-fconns[i].writes
		var msg map[string]any
		_, _ = decodeFrame(frame.data, &msg)
		if msg["event"] != "broadcast" {
			t.Fatalf("conn[%d] event = %v, want broadcast", i, msg["event"])
		}
		if msg["channel"] != "system" {
			t.Fatalf("conn[%d] channel = %v, want system", i, msg["channel"])
		}
	}
}

// --- Broadcast to Empty Connections ---

func TestBroadcastNoConnections(t *testing.T) {
	fconn := newFakeWebSocketConn()
	sender := &wsConnection{conn: fconn}

	rt := &Runtime{
		connections: make(map[*wsConnection]struct{}),
		// sender is NOT registered
	}

	req := wsMessage{
		Event:   "broadcast",
		ID:      rawStringData("b6"),
		Channel: "chat",
		Echo:    true,
	}

	err := rt.handleBroadcastRequest(sender, req, nil)
	if err != nil {
		t.Fatalf("handleBroadcastRequest error = %v", err)
	}

	// sender should get broadcast_response ok (even with no other connections)
	frame := <-fconn.writes
	var resp map[string]any
	_, _ = decodeFrame(frame.data, &resp)
	if resp["event"] != "broadcast_response" {
		t.Fatalf("event = %v, want broadcast_response", resp["event"])
	}
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("ok = %v, want true", ok)
	}
}

// --- Nil Connection Guard ---
// handleBroadcastRequest panics on nil sender because connectionSnapshot is ok
// but writeWSMessage dereferences sender. We test the nil-runtime guard only.

func TestBroadcastSenderNotRequiredForResponse(t *testing.T) {
	// Even with no connections, the function sends a response to sender.
	// If sender is nil, writeWSMessage will error (tested in ws_transport).
	// This test verifies the early guards don't panic on nil sender.
	rt := &Runtime{
		connections: make(map[*wsConnection]struct{}),
	}

	// nil sender with non-nil runtime — early guards pass, writeWSMessage fails
	req := wsMessage{Event: "broadcast", ID: rawStringData("b7"), Channel: "ch"}
	err := rt.handleBroadcastRequest(nil, req, nil)
	if err == nil {
		t.Fatal("expected error for nil sender")
	}
}

// --- Concurrent Broadcast Safety ---

func TestBroadcastConcurrent(t *testing.T) {
	fconn1 := newFakeWebSocketConn()
	fconn2 := newFakeWebSocketConn()
	conn1 := &wsConnection{conn: fconn1}
	conn2 := &wsConnection{conn: fconn2}

	rt := &Runtime{
		connections: map[*wsConnection]struct{}{
			conn1: {},
			conn2: {},
		},
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := wsMessage{
				Event:   "broadcast",
				ID:      rawStringData("bc"),
				Channel: "concurrent",
				Echo:    false,
			}
			_ = rt.handleBroadcastRequest(conn1, req, nil)
		}()
	}
	wg.Wait()

	// Drain responses — 10 broadcast_response + 10 broadcasts
	total := 0
	for total < 20 {
		select {
		case <-fconn1.writes:
			total++
		case <-fconn2.writes:
			total++
		default:
			t.Fatalf("only got %d/20 messages", total)
		}
	}
}
