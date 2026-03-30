// FILE: luminka/ws_test.go
// PURPOSE: Verify websocket protocol behavior, capability responses, watcher pushes, and origin safety rules.
// OWNS: Handler-level websocket tests for request flows and upgrade acceptance or rejection.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_luminka_runtime_safety_2026-03-30.md

package luminka

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketAppInfoAndFilesystemFlow(t *testing.T) {
	root := t.TempDir()
	rt, serverURL := newTestWebSocketRuntime(t, root, capabilityState{FS: true})
	defer rt.cleanup()

	conn := mustDialWS(t, serverURL)
	defer conn.Close()

	mustWriteWS(t, conn, map[string]any{"event": "app_info", "id": "a1"})
	appInfo := mustReadWS(t, conn)
	assertWSOK(t, appInfo, "app_info", "a1")
	if got := appInfo["name"]; got != "test-app" {
		t.Fatalf("app_info name = %v, want test-app", got)
	}
	if got := appInfo["mode"]; got != string(ModeBrowser) {
		t.Fatalf("app_info mode = %v, want %s", got, ModeBrowser)
	}
	capabilities, ok := appInfo["capabilities"].(map[string]any)
	if !ok || !truthyMapValue(capabilities, "fs") {
		t.Fatalf("app_info capabilities = %#v, want fs enabled", appInfo["capabilities"])
	}

	mustWriteWS(t, conn, map[string]any{"event": "fs_write", "id": "f1", "path": "notes/todo.txt", "data": "ship tests"})
	assertWSOK(t, mustReadWS(t, conn), "fs_response", "f1")

	mustWriteWS(t, conn, map[string]any{"event": "fs_read", "id": "f2", "path": "notes/todo.txt"})
	read := mustReadWS(t, conn)
	assertWSOK(t, read, "fs_response", "f2")
	if got := read["data"]; got != "ship tests" {
		t.Fatalf("fs_read data = %v, want ship tests", got)
	}

	mustWriteWS(t, conn, map[string]any{"event": "fs_exists", "id": "f3", "path": "notes/todo.txt"})
	exists := mustReadWS(t, conn)
	assertWSOK(t, exists, "fs_response", "f3")
	if got := exists["exists"]; got != true {
		t.Fatalf("fs_exists exists = %v, want true", got)
	}

	mustWriteWS(t, conn, map[string]any{"event": "fs_list", "id": "f4", "path": "notes"})
	list := mustReadWS(t, conn)
	assertWSOK(t, list, "fs_response", "f4")
	files, ok := list["files"].([]any)
	if !ok || len(files) != 1 || files[0] != "todo.txt" {
		t.Fatalf("fs_list files = %#v, want [todo.txt]", list["files"])
	}

	mustWriteWS(t, conn, map[string]any{"event": "fs_delete", "id": "f5", "path": "notes/todo.txt"})
	assertWSOK(t, mustReadWS(t, conn), "fs_response", "f5")

	if _, err := os.Stat(filepath.Join(root, "notes", "todo.txt")); !os.IsNotExist(err) {
		t.Fatalf("file still exists on disk, stat err = %v", err)
	}
}

func TestWebSocketCapabilityGatingResponses(t *testing.T) {
	root := t.TempDir()
	rt, serverURL := newTestWebSocketRuntime(t, root, capabilityState{})
	defer rt.cleanup()

	conn := mustDialWS(t, serverURL)
	defer conn.Close()

	mustWriteWS(t, conn, map[string]any{"event": "fs_read", "id": "fs1", "path": "blocked.txt"})
	fsResp := mustReadWS(t, conn)
	assertWSFailure(t, fsResp, "fs_response", "fs1", "filesystem capability is disabled")

	mustWriteWS(t, conn, map[string]any{"event": "script_exec", "id": "s1", "runner": "python", "file": "tool.py"})
	scriptResp := mustReadWS(t, conn)
	assertWSFailure(t, scriptResp, "script_response", "s1", "script capability is disabled")

	mustWriteWS(t, conn, map[string]any{"event": "shell_exec", "id": "h1", "cmd": "cmd"})
	shellResp := mustReadWS(t, conn)
	assertWSFailure(t, shellResp, "shell_response", "h1", "shell capability is disabled")
}

func TestWebSocketWatcherPushesFSChanged(t *testing.T) {
	root := t.TempDir()
	watchFile := filepath.Join(root, "watched.txt")
	if err := os.WriteFile(watchFile, []byte("before"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rt, serverURL := newTestWebSocketRuntime(t, root, capabilityState{FS: true})
	rt.Watcher = NewWatcher(root, 10*time.Millisecond, func(path string) error {
		return rt.pushFSChanged(path)
	})
	defer rt.cleanup()

	conn := mustDialWS(t, serverURL)
	defer conn.Close()

	mustWriteWS(t, conn, map[string]any{"event": "fs_watch", "id": "w1", "path": "watched.txt"})
	assertWSOK(t, mustReadWS(t, conn), "fs_response", "w1")

	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(watchFile, []byte("after"), 0o644); err != nil {
		t.Fatalf("WriteFile() update error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		msg := mustReadWS(t, conn)
		if msg["event"] == "fs_changed" {
			if got := msg["path"]; got != "watched.txt" {
				t.Fatalf("fs_changed path = %v, want watched.txt", got)
			}
			return
		}
	}
	t.Fatal("did not receive fs_changed notification")
}

func TestWebSocketRejectsMalformedJSONMessages(t *testing.T) {
	root := t.TempDir()
	rt, serverURL := newTestWebSocketRuntime(t, root, capabilityState{FS: true})
	defer rt.cleanup()

	conn := mustDialWS(t, serverURL)
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte("{not-json")); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}

	msg := mustReadWS(t, conn)
	if got := msg["event"]; got != "error" {
		t.Fatalf("event = %v, want error", got)
	}
	if _, ok := msg["error"].(string); !ok {
		t.Fatalf("error payload = %#v, want string", msg["error"])
	}
	if _, hasID := msg["id"]; hasID {
		t.Fatalf("id present = %#v, want omitted for malformed JSON", msg["id"])
	}
}

func TestWebSocketRejectsUnknownAndMissingEvents(t *testing.T) {
	root := t.TempDir()
	rt, serverURL := newTestWebSocketRuntime(t, root, capabilityState{FS: true})
	defer rt.cleanup()

	conn := mustDialWS(t, serverURL)
	defer conn.Close()

	mustWriteWS(t, conn, map[string]any{"event": "mystery", "id": "u1"})
	unknown := mustReadWS(t, conn)
	if got := unknown["event"]; got != "error" {
		t.Fatalf("unknown event response event = %v, want error", got)
	}
	if got := unknown["id"]; got != "u1" {
		t.Fatalf("unknown event response id = %v, want u1", got)
	}
	if got := unknown["error"]; got != `unknown event "mystery"` {
		t.Fatalf("unknown event error = %v, want unknown event message", got)
	}

	mustWriteWS(t, conn, map[string]any{"id": "u2"})
	missing := mustReadWS(t, conn)
	if got := missing["event"]; got != "error" {
		t.Fatalf("missing event response event = %v, want error", got)
	}
	if got := missing["id"]; got != "u2" {
		t.Fatalf("missing event response id = %v, want u2", got)
	}
	if got := missing["error"]; got != "message event is required" {
		t.Fatalf("missing event error = %v, want message event is required", got)
	}
}

func TestWebSocketAcceptsExactLoopbackOrigin(t *testing.T) {
	serverURL, wsURL := newOriginTestWebSocketServer(t)
	conn, _, err := dialWSWithOrigin(wsURL, serverURL)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
}

func TestWebSocketAcceptsSameLoopbackOriginHostAndPort(t *testing.T) {
	serverURL, wsURL := newOriginTestWebSocketServer(t)
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	origin := "http://" + parsed.Host

	conn, _, err := dialWSWithOrigin(wsURL, origin)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
}

func TestWebSocketRejectsForeignOrigin(t *testing.T) {
	_, wsURL := newOriginTestWebSocketServer(t)
	conn, resp, err := dialWSWithOrigin(wsURL, "https://example.com")
	if conn != nil {
		defer conn.Close()
	}
	if err == nil {
		t.Fatal("Dial() error = nil, want rejected upgrade")
	}
	if resp == nil {
		t.Fatal("response = nil, want HTTP upgrade rejection response")
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestWebSocketRejectsLoopbackOriginWithWrongPort(t *testing.T) {
	serverURL, wsURL := newOriginTestWebSocketServer(t)
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	wrongOrigin := "http://" + parsed.Hostname() + ":1"

	conn, resp, err := dialWSWithOrigin(wsURL, wrongOrigin)
	if conn != nil {
		defer conn.Close()
	}
	if err == nil {
		t.Fatal("Dial() error = nil, want rejected upgrade")
	}
	if resp == nil {
		t.Fatal("response = nil, want HTTP upgrade rejection response")
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func newTestWebSocketRuntime(t *testing.T, root string, caps capabilityState) (*Runtime, string) {
	t.Helper()
	rt := &Runtime{
		Name:         "test-app",
		Mode:         ModeBrowser,
		Root:         root,
		Capabilities: caps,
		FSBridge:     NewFSBridge(root),
		Watcher:      NewWatcher(root, 25*time.Millisecond, nil),
		ScriptBridge: NewScriptBridge(root, time.Second),
		ShellBridge:  NewShellBridge(root, time.Second),
		Assets:       fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}},
		connections:  make(map[*wsConnection]struct{}),
		shutdownCh:   make(chan struct{}),
	}
	rt.Watcher = NewWatcher(root, 25*time.Millisecond, func(path string) error {
		return rt.pushFSChanged(path)
	})

	server := httptest.NewServer(http.HandlerFunc(rt.serveWebSocket))
	t.Cleanup(server.Close)

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	parsed.Scheme = "ws"
	return rt, parsed.String()
}

func newOriginTestWebSocketServer(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	rt := &Runtime{
		Name:         "origin-test-app",
		Mode:         ModeBrowser,
		Root:         root,
		Capabilities: capabilityState{FS: true},
		FSBridge:     NewFSBridge(root),
		Watcher:      NewWatcher(root, 25*time.Millisecond, nil),
		ScriptBridge: NewScriptBridge(root, time.Second),
		ShellBridge:  NewShellBridge(root, time.Second),
		Assets:       fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}},
		connections:  make(map[*wsConnection]struct{}),
		shutdownCh:   make(chan struct{}),
	}

	server := httptest.NewServer(http.HandlerFunc(rt.serveWebSocket))
	t.Cleanup(func() {
		server.Close()
		_ = rt.cleanup()
	})

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	parsed.Scheme = "ws"
	return server.URL, parsed.String()
}

func mustDialWS(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	headers := http.Header{}
	headers.Set("Origin", "http://"+parsed.Host)
	conn, _, err := websocket.DefaultDialer.Dial(serverURL, headers)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	return conn
}

func dialWSWithOrigin(serverURL string, origin string) (*websocket.Conn, *http.Response, error) {
	headers := http.Header{}
	headers.Set("Origin", origin)
	return websocket.DefaultDialer.Dial(serverURL, headers)
}

func mustWriteWS(t *testing.T, conn *websocket.Conn, payload map[string]any) {
	t.Helper()
	if err := conn.WriteJSON(payload); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
}

func mustReadWS(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; data=%s", err, string(data))
	}
	return msg
}

func assertWSOK(t *testing.T, msg map[string]any, event string, id string) {
	t.Helper()
	if got := msg["event"]; got != event {
		t.Fatalf("event = %v, want %s", got, event)
	}
	if got := msg["id"]; got != id {
		t.Fatalf("id = %v, want %s", got, id)
	}
	if got := msg["ok"]; got != true {
		t.Fatalf("ok = %v, want true", got)
	}
}

func assertWSFailure(t *testing.T, msg map[string]any, event string, id string, errContains string) {
	t.Helper()
	if got := msg["event"]; got != event {
		t.Fatalf("event = %v, want %s", got, event)
	}
	if got := msg["id"]; got != id {
		t.Fatalf("id = %v, want %s", got, id)
	}
	if got := msg["ok"]; got != false {
		t.Fatalf("ok = %v, want false", got)
	}
	errText, _ := msg["error"].(string)
	if errText != errContains {
		t.Fatalf("error = %q, want %q", errText, errContains)
	}
}

func truthyMapValue(values map[string]any, key string) bool {
	v, _ := values[key].(bool)
	return v
}

var _ fs.FS = fstest.MapFS{}
