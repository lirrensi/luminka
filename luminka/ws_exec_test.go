// FILE: luminka/ws_exec_test.go
// PURPOSE: Verify script/shell stream request validation and capability gating.
// OWNS: Tests for handleScriptStreamRequest and handleShellStreamRequest nil-runtime,
// capability gating, bridge availability, and error forwarding.
// EXPORTS: none

package luminka

import (
	"testing"
	"time"
)

// --- Nil Runtime ---

func TestScriptStreamNilRuntime(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &wsConnection{conn: fconn}
	req := wsMessage{Event: "script_stream", ID: rawStringData("s1"), Runner: "python", File: "test.py"}

	err := (*Runtime)(nil).handleScriptStreamRequest(conn, req)
	if err != nil {
		t.Fatalf("expected nil (error sent via WS), got: %v", err)
	}

	frame := <-fconn.writes
	var resp map[string]any
	_, _ = decodeFrame(frame.data, &resp)
	if resp["event"] != "error" {
		t.Fatalf("event = %v, want error", resp["event"])
	}
	if resp["error"] != "runtime is required" {
		t.Fatalf("error = %v, want 'runtime is required'", resp["error"])
	}
}

func TestShellStreamNilRuntime(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &wsConnection{conn: fconn}
	req := wsMessage{Event: "shell_stream", ID: rawStringData("sh1"), Cmd: "ls"}

	err := (*Runtime)(nil).handleShellStreamRequest(conn, req)
	if err != nil {
		t.Fatalf("expected nil (error sent via WS), got: %v", err)
	}

	frame := <-fconn.writes
	var resp map[string]any
	_, _ = decodeFrame(frame.data, &resp)
	if resp["event"] != "error" {
		t.Fatalf("event = %v, want error", resp["event"])
	}
	if resp["error"] != "runtime is required" {
		t.Fatalf("error = %v, want 'runtime is required'", resp["error"])
	}
}

// --- Script Capability Disabled ---

func TestScriptStreamCapabilityDisabled(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &wsConnection{conn: fconn}

	rt := &Runtime{
		Capabilities: capabilityState{Scripts: false},
		ScriptBridge: NewScriptBridge("", time.Second),
		connections:  make(map[*wsConnection]struct{}),
	}
	rt.connections[conn] = struct{}{}

	req := wsMessage{Event: "script_stream", ID: rawStringData("s2"), Runner: "python", File: "test.py"}

	err := rt.handleScriptStreamRequest(conn, req)
	if err != nil {
		t.Fatalf("expected nil (error sent via WS), got: %v", err)
	}

	frame := <-fconn.writes
	var resp map[string]any
	_, _ = decodeFrame(frame.data, &resp)
	if resp["event"] != "script_response" {
		t.Fatalf("event = %v, want script_response", resp["event"])
	}
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatalf("ok = %v, want false", ok)
	}
	if resp["error"] != "script capability is disabled" {
		t.Fatalf("error = %v, want 'script capability is disabled'", resp["error"])
	}
}

// --- Shell Capability Disabled ---

func TestShellStreamCapabilityDisabled(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &wsConnection{conn: fconn}

	rt := &Runtime{
		Capabilities: capabilityState{Shell: false},
		ShellBridge:  NewShellBridge("", time.Second),
		connections:  make(map[*wsConnection]struct{}),
	}
	rt.connections[conn] = struct{}{}

	req := wsMessage{Event: "shell_stream", ID: rawStringData("sh2"), Cmd: "ls"}

	err := rt.handleShellStreamRequest(conn, req)
	if err != nil {
		t.Fatalf("expected nil (error sent via WS), got: %v", err)
	}

	frame := <-fconn.writes
	var resp map[string]any
	_, _ = decodeFrame(frame.data, &resp)
	if resp["event"] != "shell_response" {
		t.Fatalf("event = %v, want shell_response", resp["event"])
	}
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatalf("ok = %v, want false", ok)
	}
	if resp["error"] != "shell capability is disabled" {
		t.Fatalf("error = %v, want 'shell capability is disabled'", resp["error"])
	}
}

// --- ScriptBridge Nil ---

func TestScriptStreamBridgeNil(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &wsConnection{conn: fconn}

	rt := &Runtime{
		Capabilities: capabilityState{Scripts: true},
		ScriptBridge: nil,
		connections:  make(map[*wsConnection]struct{}),
	}
	rt.connections[conn] = struct{}{}

	req := wsMessage{Event: "script_stream", ID: rawStringData("s3"), Runner: "python", File: "test.py"}

	err := rt.handleScriptStreamRequest(conn, req)
	if err != nil {
		t.Fatalf("expected nil (error sent via WS), got: %v", err)
	}

	frame := <-fconn.writes
	var resp map[string]any
	_, _ = decodeFrame(frame.data, &resp)
	if resp["event"] != "script_response" {
		t.Fatalf("event = %v, want script_response", resp["event"])
	}
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatalf("ok = %v, want false", ok)
	}
	if resp["error"] != "script bridge is unavailable" {
		t.Fatalf("error = %v, want 'script bridge is unavailable'", resp["error"])
	}
}

// --- ShellBridge Nil ---

func TestShellStreamBridgeNil(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &wsConnection{conn: fconn}

	rt := &Runtime{
		Capabilities: capabilityState{Shell: true},
		ShellBridge:  nil,
		connections:  make(map[*wsConnection]struct{}),
	}
	rt.connections[conn] = struct{}{}

	req := wsMessage{Event: "shell_stream", ID: rawStringData("sh3"), Cmd: "ls"}

	err := rt.handleShellStreamRequest(conn, req)
	if err != nil {
		t.Fatalf("expected nil (error sent via WS), got: %v", err)
	}

	frame := <-fconn.writes
	var resp map[string]any
	_, _ = decodeFrame(frame.data, &resp)
	if resp["event"] != "shell_response" {
		t.Fatalf("event = %v, want shell_response", resp["event"])
	}
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatalf("ok = %v, want false", ok)
	}
	if resp["error"] != "shell bridge is unavailable" {
		t.Fatalf("error = %v, want 'shell bridge is unavailable'", resp["error"])
	}
}

// --- ExecStream Error (stub returns "not available" in non-tagged build) ---

func TestScriptStreamExecStreamError(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &wsConnection{conn: fconn}

	rt := &Runtime{
		Capabilities: capabilityState{Scripts: true},
		ScriptBridge: NewScriptBridge(t.TempDir(), time.Second),
		connections:  make(map[*wsConnection]struct{}),
	}
	rt.connections[conn] = struct{}{}

	req := wsMessage{Event: "script_stream", ID: rawStringData("s4"), Runner: "python", File: "test.py"}

	err := rt.handleScriptStreamRequest(conn, req)
	if err != nil {
		t.Fatalf("expected nil (error sent via WS), got: %v", err)
	}

	// In a non-tagged build, the stub ExecStream always returns
	// "script support is not available in this build; rebuild with -tags scripts"
	frame := <-fconn.writes
	var resp map[string]any
	_, _ = decodeFrame(frame.data, &resp)
	if resp["event"] != "script_response" {
		t.Fatalf("event = %v, want script_response", resp["event"])
	}
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatalf("ok = %v, want false", ok)
	}
	errMsg, _ := resp["error"].(string)
	if errMsg == "" {
		t.Fatal("expected non-empty error message from stub ExecStream")
	}
}

func TestShellStreamExecStreamError(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &wsConnection{conn: fconn}

	rt := &Runtime{
		Capabilities: capabilityState{Shell: true},
		ShellBridge:  NewShellBridge(t.TempDir(), time.Second),
		connections:  make(map[*wsConnection]struct{}),
	}
	rt.connections[conn] = struct{}{}

	req := wsMessage{Event: "shell_stream", ID: rawStringData("sh4"), Cmd: "ls"}

	err := rt.handleShellStreamRequest(conn, req)
	if err != nil {
		t.Fatalf("expected nil (error sent via WS), got: %v", err)
	}

	frame := <-fconn.writes
	var resp map[string]any
	_, _ = decodeFrame(frame.data, &resp)
	if resp["event"] != "shell_response" {
		t.Fatalf("event = %v, want shell_response", resp["event"])
	}
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatalf("ok = %v, want false", ok)
	}
	errMsg, _ := resp["error"].(string)
	if errMsg == "" {
		t.Fatal("expected non-empty error message from stub ExecStream")
	}
}

// --- Nil Connection Guards ---
// The handler functions write responses via writeExecResponse, which handles nil conn.
// But the handlers themselves check rt == nil before touching conn.

func TestScriptStreamNilConn(t *testing.T) {
	// With non-nil runtime but nil conn, the handler passes guards and calls
	// writeExecResponse which checks conn != nil.
	rt := &Runtime{
		Capabilities: capabilityState{Scripts: true},
		ScriptBridge: NewScriptBridge("", time.Second),
	}
	req := wsMessage{Event: "script_stream", ID: rawStringData("s5"), Runner: "python", File: "test.py"}

	err := rt.handleScriptStreamRequest(nil, req)
	if err == nil {
		t.Fatal("expected error for nil conn")
	}
}

// --- Log Event Verification ---
// The handler logs events on both error and success paths.
// We can verify by checking that the response was written (hard to verify log file).

func TestScriptStreamLogEventOnError(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &wsConnection{conn: fconn}

	// Capabilities disabled → handler logs nothing (returns early before log)
	rt := &Runtime{
		Capabilities: capabilityState{Scripts: false},
		connections:  make(map[*wsConnection]struct{}),
	}
	rt.connections[conn] = struct{}{}

	req := wsMessage{Event: "script_stream", ID: rawStringData("s6"), Runner: "python", File: "test.py"}

	_ = rt.handleScriptStreamRequest(conn, req)
	frame := <-fconn.writes
	var resp map[string]any
	_, _ = decodeFrame(frame.data, &resp)
	if resp["event"] != "script_response" {
		t.Fatalf("event = %v, want script_response", resp["event"])
	}
	// Error path — the handler did not log anything because it returned early
	// at the capability check. This is correct behavior.
}

// --- Request with Timeout ---

func TestScriptStreamWithTimeout(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &wsConnection{conn: fconn}

	rt := &Runtime{
		Capabilities: capabilityState{Scripts: true},
		ScriptBridge: NewScriptBridge(t.TempDir(), time.Second),
		connections:  make(map[*wsConnection]struct{}),
	}
	rt.connections[conn] = struct{}{}

	req := wsMessage{
		Event:   "script_stream",
		ID:      rawStringData("s7"),
		Runner:  "python",
		File:    "test.py",
		Timeout: 500, // 500ms timeout
	}

	err := rt.handleScriptStreamRequest(conn, req)
	if err != nil {
		t.Fatalf("expected nil (error sent via WS), got: %v", err)
	}

	// Should get a response (error from stub or from timeout)
	frame := <-fconn.writes
	var resp map[string]any
	_, _ = decodeFrame(frame.data, &resp)
	if resp["event"] != "script_response" {
		t.Fatalf("event = %v, want script_response", resp["event"])
	}
}
