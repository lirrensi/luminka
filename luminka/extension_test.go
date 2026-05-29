// FILE: luminka/extension_test.go
// PURPOSE: Verify the Extension API types and behaviors.
// OWNS: Tests for RegisterHandler, HandlerFunc, ErrUnhandled, WithOverride.
// EXPORTS: none
// DOCS: docs/spec.md

package luminka

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// ErrUnhandled sentinel
// ---------------------------------------------------------------------------

func TestErrUnhandledSentinel(t *testing.T) {
	if !errors.Is(ErrUnhandled, ErrUnhandled) {
		t.Fatal("ErrUnhandled must be identifiable via errors.Is()")
	}
	if ErrUnhandled.Error() != "event not handled by custom handler" {
		t.Fatalf("ErrUnhandled.Error() = %q, want %q",
			ErrUnhandled.Error(), "event not handled by custom handler")
	}
}

func TestErrUnhandledAllowsFallthrough(t *testing.T) {
	// Simulate a handler that declines to handle the event.
	handler := HandlerFunc(func(conn *WSConnection, msg *WSMessage) error {
		return ErrUnhandled
	})
	err := handler(nil, &WSMessage{Event: "fallthrough:test"})
	if !errors.Is(err, ErrUnhandled) {
		t.Fatal("handler returning ErrUnhandled should propagate it")
	}
}

// ---------------------------------------------------------------------------
// WithOverride option
// ---------------------------------------------------------------------------

func TestWithOverride(t *testing.T) {
	cfg := &registerConfig{}
	if cfg.override {
		t.Fatal("registerConfig.override should default to false")
	}
	WithOverride()(cfg)
	if !cfg.override {
		t.Fatal("WithOverride() should set override to true")
	}
}

func TestWithOverrideIdempotent(t *testing.T) {
	cfg := &registerConfig{}
	WithOverride()(cfg)
	WithOverride()(cfg) // applying twice should still be true
	if !cfg.override {
		t.Fatal("override should remain true after two applications")
	}
}

// ---------------------------------------------------------------------------
// RegisterHandler storage
// ---------------------------------------------------------------------------

func TestRegisterHandlerStoresHandler(t *testing.T) {
	rt := &Runtime{
		customHandlers: make(map[string]handlerEntry),
	}

	called := false
	handler := HandlerFunc(func(conn *WSConnection, msg *WSMessage) error {
		called = true
		return nil
	})

	rt.RegisterHandler("custom:event", handler)

	entry, ok := rt.customHandlers["custom:event"]
	if !ok {
		t.Fatal("RegisterHandler did not store the handler entry")
	}
	if entry.handler == nil {
		t.Fatal("stored handler entry has nil handler")
	}
	if entry.override {
		t.Fatal("handler without WithOverride should have override=false")
	}

	// Call the stored handler to confirm it works.
	err := entry.handler(nil, &WSMessage{Event: "custom:event"})
	if err != nil {
		t.Fatalf("stored handler returned error: %v", err)
	}
	if !called {
		t.Fatal("stored handler was not invoked")
	}
}

func TestRegisterHandlerWithOverride(t *testing.T) {
	rt := &Runtime{
		customHandlers: make(map[string]handlerEntry),
	}

	handler := HandlerFunc(func(conn *WSConnection, msg *WSMessage) error {
		return nil
	})

	rt.RegisterHandler("override:event", handler, WithOverride())

	entry, ok := rt.customHandlers["override:event"]
	if !ok {
		t.Fatal("RegisterHandler with WithOverride did not store handler")
	}
	if !entry.override {
		t.Fatal("handler registered with WithOverride should have override=true")
	}
}

func TestRegisterHandlerInitializesNilMap(t *testing.T) {
	rt := &Runtime{
		customHandlers: nil, // explicitly nil
	}

	handler := HandlerFunc(func(conn *WSConnection, msg *WSMessage) error {
		return nil
	})

	rt.RegisterHandler("nil:map", handler)

	entry, ok := rt.customHandlers["nil:map"]
	if !ok {
		t.Fatal("RegisterHandler should initialize nil map and store handler")
	}
	if entry.handler == nil {
		t.Fatal("stored handler should not be nil")
	}
}

func TestRegisterHandlerVariadicNoOpts(t *testing.T) {
	rt := &Runtime{
		customHandlers: make(map[string]handlerEntry),
	}

	handler := HandlerFunc(func(conn *WSConnection, msg *WSMessage) error {
		return nil
	})

	// Passing no opts should be valid.
	rt.RegisterHandler("no:opts", handler)
	if rt.customHandlers["no:opts"].override {
		t.Fatal("handler with no opts should have override=false")
	}
}

func TestRegisterHandlerOverwritesExisting(t *testing.T) {
	rt := &Runtime{
		customHandlers: make(map[string]handlerEntry),
	}

	var calls []string
	h1 := HandlerFunc(func(_ *WSConnection, _ *WSMessage) error {
		calls = append(calls, "h1")
		return nil
	})
	h2 := HandlerFunc(func(_ *WSConnection, _ *WSMessage) error {
		calls = append(calls, "h2")
		return nil
	})

	rt.RegisterHandler("same:event", h1)
	rt.RegisterHandler("same:event", h2, WithOverride())

	entry := rt.customHandlers["same:event"]
	_ = entry.handler(nil, &WSMessage{Event: "same:event"})

	if len(calls) != 1 || calls[0] != "h2" {
		t.Fatalf("expected only h2 to be called, got %v", calls)
	}
	if !entry.override {
		t.Fatal("overwritten entry should have override=true")
	}
}

func TestRegisterHandlerConcurrentSafe(t *testing.T) {
	rt := &Runtime{
		customHandlers: make(map[string]handlerEntry),
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			handler := HandlerFunc(func(_ *WSConnection, _ *WSMessage) error {
				return ErrUnhandled
			})
			rt.RegisterHandler("concurrent:test", handler)
		}(i)
	}
	wg.Wait()

	// After all goroutines, exactly one entry should exist.
	entry, ok := rt.customHandlers["concurrent:test"]
	if !ok {
		t.Fatal("concurrent RegisterHandler did not produce an entry")
	}
	if entry.handler == nil {
		t.Fatal("concurrent registration produced nil handler")
	}
}

// ---------------------------------------------------------------------------
// Integration: custom handler dispatch preempts built-in switch
// ---------------------------------------------------------------------------

func TestCustomHandlerPreemptsBuiltinAppInfo(t *testing.T) {
	root := t.TempDir()
	rt, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	customCalled := false
	rt.RegisterHandler("app:info", HandlerFunc(func(c *WSConnection, msg *WSMessage) error {
		customCalled = true
		// Return a minimal custom response instead of the built-in one.
		return WriteWSMessage(c, WSMessage{
			Event: "response:app:info",
			ID:    msg.ID,
			Ok:    boolPtr(true),
			Data:  json.RawMessage(`"custom-handler"`),
		})
	}))

	mustWriteWS(t, conn, map[string]any{"event": "app:info", "id": "c1"})
	resp := mustReadWS(t, conn)

	if !customCalled {
		t.Fatal("custom handler was not invoked")
	}
	if resp["event"] != "response:app:info" {
		t.Fatalf("event = %v, want response:app:info", resp["event"])
	}
	if resp["data"] != "custom-handler" {
		t.Fatalf("data = %v, want custom-handler", resp["data"])
	}
	// Built-in app:info also returns name, capabilities, etc.
	// If the custom handler preempts, those built-in fields should NOT be present.
	if _, hasName := resp["name"]; hasName {
		t.Fatal("custom handler did not preempt built-in: name field present from built-in")
	}
}

func TestCustomHandlerErrUnhandledFallsThroughToBuiltin(t *testing.T) {
	root := t.TempDir()
	rt, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	rt.RegisterHandler("app:info", HandlerFunc(func(_ *WSConnection, _ *WSMessage) error {
		return ErrUnhandled // decline to handle
	}))

	mustWriteWS(t, conn, map[string]any{"event": "app:info", "id": "ft1"})
	resp := mustReadWS(t, conn)

	// Built-in handler should have produced the full response.
	if resp["event"] != "response:app:info" {
		t.Fatalf("event = %v, want response:app:info", resp["event"])
	}
	if resp["data"] != nil {
		t.Fatalf("data should be nil from built-in, got %v", resp["data"])
	}
	if name, ok := resp["name"]; !ok || name != "test-app" {
		t.Fatalf("name = %v, want test-app (built-in response)", name)
	}
}

func TestCustomHandlerErrorReturnsErrorResponse(t *testing.T) {
	root := t.TempDir()
	rt, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	rt.RegisterHandler("app:info", HandlerFunc(func(_ *WSConnection, _ *WSMessage) error {
		return errors.New("custom handler failure")
	}))

	mustWriteWS(t, conn, map[string]any{"event": "app:info", "id": "e1"})
	resp := mustReadWS(t, conn)

	if resp["event"] != "response:error" {
		t.Fatalf("event = %v, want response:error", resp["event"])
	}
	if errMsg, ok := resp["error"].(string); !ok || errMsg != "custom handler failure" {
		t.Fatalf("error = %v, want custom handler failure", resp["error"])
	}
}

// ---------------------------------------------------------------------------
// Namespacing: ensure unknown namespaced events are still rejected
// ---------------------------------------------------------------------------

func TestWebSocketRejectsUnknownNamespacedEvents(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	// A namespaced event that does not exist in the switch.
	mustWriteWS(t, conn, map[string]any{"event": "file:nonexistent", "id": "ns1"})
	resp := mustReadWS(t, conn)
	if resp["event"] != "response:error" {
		t.Fatalf("event = %v, want response:error", resp["event"])
	}
	if got, ok := resp["error"].(string); !ok || got != `unknown event "file:nonexistent"` {
		t.Fatalf("error = %q, want unknown event message", got)
	}

	// A namespaced event with valid prefix but invalid action.
	mustWriteWS(t, conn, map[string]any{"event": "stream:invalid_action", "id": "ns2"})
	resp2 := mustReadWS(t, conn)
	if resp2["event"] != "response:error" {
		t.Fatalf("event = %v, want response:error", resp2["event"])
	}

	// An old-style flat event that should no longer be recognized.
	mustWriteWS(t, conn, map[string]any{"event": "fs_read_file", "id": "ns3"})
	resp3 := mustReadWS(t, conn)
	if resp3["event"] != "response:error" {
		t.Fatalf("event = %v, want response:error for old-style event", resp3["event"])
	}
	if got, ok := resp3["error"].(string); !ok || got != `unknown event "fs_read_file"` {
		t.Fatalf("error = %q, want unknown event for flat name", got)
	}
}
