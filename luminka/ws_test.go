// FILE: luminka/ws_test.go
// PURPOSE: Verify binary websocket protocol behavior, capability responses, watcher pushes, and origin safety rules.
// OWNS: Handler-level websocket tests for request flows and origin validation.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketAppInfoAndFilesystemFlow(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

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

	mustWriteWS(t, conn, map[string]any{"event": "fs_write_text", "id": "f1", "path": "notes/todo.txt", "data": "ship tests"})
	assertWSOK(t, mustReadWS(t, conn), "fs_response", "f1")

	mustWriteWS(t, conn, map[string]any{"event": "fs_read_text", "id": "f2", "path": "notes/todo.txt"})
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

func TestWebSocketChunkedByteFilesystemFlow(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	payload := bytes.Repeat([]byte("0123456789abcdef"), (fsStreamChunkSize/16)+2)

	mustWriteWS(t, conn, map[string]any{"event": "fs_open_write", "id": "bw1", "path": "bytes/payload.bin"})
	writeAck, _ := mustReadWSFrame(t, conn)
	streamID, _ := writeAck["stream_id"].(string)
	if streamID == "" {
		t.Fatal("fs_open_write ack missing stream_id")
	}

	mustWriteWSFrame(t, conn, map[string]any{"event": "stream_chunk", "stream_id": streamID, "seq": 0}, payload[:fsStreamChunkSize])
	mustWriteWSFrame(t, conn, map[string]any{"event": "stream_chunk", "stream_id": streamID, "seq": 1}, payload[fsStreamChunkSize:])
	mustWriteWS(t, conn, map[string]any{"event": "stream_close", "id": "bw-close", "stream_id": streamID})
	assertWSOK(t, mustReadWS(t, conn), "stream_close", "bw-close")

	mustWriteWS(t, conn, map[string]any{"event": "fs_open_read", "id": "br1", "path": "bytes/payload.bin"})
	readAck, _ := mustReadWSFrame(t, conn)
	readStreamID, _ := readAck["stream_id"].(string)
	if readStreamID == "" {
		t.Fatal("fs_open_read ack missing stream_id")
	}

	var got bytes.Buffer
	for {
		header, chunk := mustReadWSFrame(t, conn)
		switch header["event"] {
		case "stream_chunk":
			got.Write(chunk)
		case "stream_close":
			if sid, _ := header["stream_id"].(string); sid != readStreamID {
				t.Fatalf("stream_close stream_id = %q, want %q", sid, readStreamID)
			}
			if !bytes.Equal(got.Bytes(), payload) {
				t.Fatalf("read payload = %d bytes, want %d", got.Len(), len(payload))
			}
			return
		default:
			t.Fatalf("unexpected event %q", header["event"])
		}
	}
}

func TestWebSocketCapabilityGatingResponses(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{})

	mustWriteWS(t, conn, map[string]any{"event": "fs_read_text", "id": "fs1", "path": "blocked.txt"})
	fsResp := mustReadWS(t, conn)
	assertWSFailure(t, fsResp, "fs_response", "fs1", "filesystem capability is disabled")

	mustWriteWS(t, conn, map[string]any{"event": "fs_open_read", "id": "fs2", "path": "blocked.txt"})
	openReadResp := mustReadWS(t, conn)
	assertWSFailure(t, openReadResp, "fs_response", "fs2", "filesystem capability is disabled")

	mustWriteWS(t, conn, map[string]any{"event": "fs_open_write", "id": "fs3", "path": "blocked.txt"})
	openWriteResp := mustReadWS(t, conn)
	assertWSFailure(t, openWriteResp, "fs_response", "fs3", "filesystem capability is disabled")

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

	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

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

func TestWebSocketRejectsTextFrames(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	mustWriteTextWS(t, conn, []byte(`{"event":"app_info","id":"t1"}`))
	msg := mustReadWS(t, conn)
	if got := msg["event"]; got != "error" {
		t.Fatalf("event = %v, want error", got)
	}
	if _, ok := msg["error"].(string); !ok {
		t.Fatalf("error payload = %#v, want string", msg["error"])
	}
	if _, hasID := msg["id"]; hasID {
		t.Fatalf("id present = %#v, want omitted for text frame rejection", msg["id"])
	}
}

func TestWebSocketRejectsMalformedBinaryFrames(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	conn.reads <- fakeWSFrame{msgType: websocket.BinaryMessage, data: []byte{1, 2, 3}}
	msg := mustReadWS(t, conn)
	if got := msg["event"]; got != "error" {
		t.Fatalf("event = %v, want error", got)
	}
	if _, ok := msg["error"].(string); !ok {
		t.Fatalf("error payload = %#v, want string", msg["error"])
	}
}

func TestWebSocketRejectsUnknownAndMissingEvents(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

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
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Host = "localhost:1234"
	req.Header.Set("Origin", "http://localhost:1234")
	if !websocketOriginAllowed(req) {
		t.Fatal("websocketOriginAllowed() = false, want true")
	}
}

func TestWebSocketAcceptsSameLoopbackOriginHostAndPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Host = "127.0.0.1:1234"
	req.Header.Set("Origin", "http://127.0.0.1:1234")
	if !websocketOriginAllowed(req) {
		t.Fatal("websocketOriginAllowed() = false, want true")
	}
}

func TestWebSocketRejectsForeignOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Host = "127.0.0.1:1234"
	req.Header.Set("Origin", "https://example.com")
	if websocketOriginAllowed(req) {
		t.Fatal("websocketOriginAllowed() = true, want false")
	}
}

func TestWebSocketRejectsLoopbackOriginWithWrongPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Host = "127.0.0.1:1234"
	req.Header.Set("Origin", "http://127.0.0.1:1")
	if websocketOriginAllowed(req) {
		t.Fatal("websocketOriginAllowed() = true, want false")
	}
}

func newTestWebSocketRuntime(t *testing.T, root string, caps capabilityState) (*Runtime, *fakeWebSocketConn) {
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
		streams:      newStreamRegistry(),
		connections:  make(map[*wsConnection]struct{}),
		shutdownCh:   make(chan struct{}),
	}
	rt.Watcher = NewWatcher(root, 25*time.Millisecond, func(path string) error {
		return rt.pushFSChanged(path)
	})

	conn := newFakeWebSocketConn()
	wsConn := rt.registerConnection(conn)
	done := make(chan struct{})
	go func() {
		rt.handleWebSocketSession(wsConn)
		close(done)
	}()

	t.Cleanup(func() {
		conn.CloseInput()
		<-done
		rt.unregisterConnection(wsConn)
		_ = rt.cleanup()
	})

	return rt, conn
}

type fakeWebSocketConn struct {
	reads  chan fakeWSFrame
	writes chan fakeWSFrame
	closed chan struct{}
	once   sync.Once
}

type fakeWSFrame struct {
	msgType int
	data    []byte
	err     error
}

func newFakeWebSocketConn() *fakeWebSocketConn {
	return &fakeWebSocketConn{
		reads:  make(chan fakeWSFrame, 16),
		writes: make(chan fakeWSFrame, 16),
		closed: make(chan struct{}),
	}
}

func (f *fakeWebSocketConn) ReadMessage() (int, []byte, error) {
	select {
	case frame, ok := <-f.reads:
		if !ok {
			return 0, nil, io.EOF
		}
		return frame.msgType, append([]byte(nil), frame.data...), frame.err
	case <-f.closed:
		return 0, nil, io.EOF
	}
}

func (f *fakeWebSocketConn) WriteMessage(msgType int, data []byte) error {
	frame := fakeWSFrame{msgType: msgType, data: append([]byte(nil), data...)}
	select {
	case f.writes <- frame:
		return nil
	case <-f.closed:
		return io.EOF
	}
}

func (f *fakeWebSocketConn) Close() error {
	f.CloseInput()
	return nil
}

func (f *fakeWebSocketConn) CloseInput() {
	f.once.Do(func() {
		close(f.closed)
		close(f.reads)
	})
}

func mustWriteWS(t *testing.T, conn *fakeWebSocketConn, payload map[string]any) {
	t.Helper()
	data, err := encodeFrame(payload, nil)
	if err != nil {
		t.Fatalf("encodeFrame() error = %v", err)
	}
	conn.reads <- fakeWSFrame{msgType: websocket.BinaryMessage, data: data}
}

func mustWriteTextWS(t *testing.T, conn *fakeWebSocketConn, data []byte) {
	t.Helper()
	conn.reads <- fakeWSFrame{msgType: websocket.TextMessage, data: append([]byte(nil), data...)}
}

func mustReadWS(t *testing.T, conn *fakeWebSocketConn) map[string]any {
	t.Helper()
	select {
	case frame := <-conn.writes:
		if frame.msgType != websocket.BinaryMessage {
			t.Fatalf("message type = %d, want binary", frame.msgType)
		}
		var msg map[string]any
		if _, err := decodeFrame(frame.data, &msg); err != nil {
			t.Fatalf("decodeFrame() error = %v; data=%s", err, string(frame.data))
		}
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket response")
	}
	return nil
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
