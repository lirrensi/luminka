// FILE: luminka/fs_stream_test.go
// PURPOSE: Verify byte-first filesystem streams over websocket transport.
// OWNS: Chunked file write and read coverage for stream-open protocol events.
// EXPORTS: TestFilesystemStreamWriteAndReadRoundTrip
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestFilesystemStreamWriteAndReadRoundTrip(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	payload := bytes.Repeat([]byte("0123456789abcdef"), (fsStreamChunkSize/16)+2)

	mustWriteWS(t, conn, map[string]any{"event": "fs_open_write", "id": "open-write", "path": "bytes/payload.bin"})
	writeAck, _ := mustReadWSFrame(t, conn)
	writeStreamID, _ := writeAck["stream_id"].(string)
	if writeStreamID == "" {
		t.Fatal("fs_open_write ack missing stream_id")
	}

	mustWriteWSFrame(t, conn, map[string]any{"event": "stream_chunk", "stream_id": writeStreamID, "seq": 0}, payload[:fsStreamChunkSize])
	mustWriteWSFrame(t, conn, map[string]any{"event": "stream_chunk", "stream_id": writeStreamID, "seq": 1}, payload[fsStreamChunkSize:])
	mustWriteWS(t, conn, map[string]any{"event": "stream_close", "id": "write-close", "stream_id": writeStreamID})
	assertWSOK(t, mustReadWS(t, conn), "stream_close", "write-close")

	mustWriteWS(t, conn, map[string]any{"event": "fs_open_read", "id": "open-read", "path": "bytes/payload.bin"})
	readAck, _ := mustReadWSFrame(t, conn)
	readStreamID, _ := readAck["stream_id"].(string)
	if readStreamID == "" {
		t.Fatal("fs_open_read ack missing stream_id")
	}

	var got bytes.Buffer
	deadline := time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for stream read frames")
		}
		header, chunk := mustReadWSFrame(t, conn)
		switch header["event"] {
		case "stream_chunk":
			if sid, _ := header["stream_id"].(string); sid != readStreamID {
				t.Fatalf("stream_chunk stream_id = %q, want %q", sid, readStreamID)
			}
			got.Write(chunk)
		case "stream_close":
			if got := header["ok"]; got != true {
				t.Fatalf("stream_close ok = %v, want true", got)
			}
			if got := header["event"]; got != "stream_close" {
				t.Fatalf("stream_close event = %v, want stream_close", got)
			}
			if sid, _ := header["stream_id"].(string); sid != readStreamID {
				t.Fatalf("stream_close stream_id = %q, want %q", sid, readStreamID)
			}
			if !bytes.Equal(got.Bytes(), payload) {
				t.Fatalf("streamed payload mismatch: got %d bytes, want %d", got.Len(), len(payload))
			}
			return
		default:
			t.Fatalf("unexpected event %q", header["event"])
		}
	}
}

func TestFilesystemStreamOpenRejectsPathEscape(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	mustWriteWS(t, conn, map[string]any{"event": "fs_open_read", "id": "escape", "path": filepath.Join("..", "escape.bin")})
	resp := mustReadWS(t, conn)
	assertWSFailure(t, resp, "fs_response", "escape", "path escapes root")
}

// ---------------------------------------------------------------------------
// Wire-level tests for handle operations (read/write/close/stat/truncate/sync/chmod/utimes)
// ---------------------------------------------------------------------------

func TestHandleWireRoundTrip(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	// Create a file first
	content := "hello handle world"
	if err := os.WriteFile(filepath.Join(root, "handle_test.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Open handle
	mustWriteWS(t, conn, map[string]any{"event": "fs_open", "id": "open1", "path": "handle_test.txt", "flag": "r+"})
	openResp := mustReadWS(t, conn)
	if ok, _ := openResp["ok"].(bool); !ok {
		t.Fatalf("fs_open response ok = false: %v", openResp)
	}
	handleID, ok := openResp["handle_id"].(string)
	if !ok {
		// The response might use "stream_id" instead — check both
		handleID, ok = openResp["stream_id"].(string)
		if !ok {
			t.Fatal("open response missing handle_id and stream_id")
		}
	}

	// Read from handle
	mustWriteWS(t, conn, map[string]any{"event": "handle_read", "stream_id": handleID, "len": 0, "perm": 0})
	readResp, readPayload := mustReadWSFrame(t, conn)
	if readResp["event"] != "fs_response" || readResp["ok"] != true {
		t.Fatalf("handle_read response = %v, want fs_response ok", readResp)
	}
	if string(readPayload) != content {
		t.Fatalf("handle_read payload = %q, want %q", string(readPayload), content)
	}

	// Stat handle
	mustWriteWS(t, conn, map[string]any{"event": "handle_stat", "stream_id": handleID})
	statResp := mustReadWS(t, conn)
	if ok, _ := statResp["ok"].(bool); !ok {
		t.Fatalf("handle_stat response ok = false: %v", statResp)
	}
	_, hasStat := statResp["stat"]
	if !hasStat {
		_, hasStat = statResp["data"]
	}
	if !hasStat {
		t.Fatalf("handle_stat response has no stat field: %v", statResp)
	}

	// Truncate handle
	mustWriteWS(t, conn, map[string]any{"event": "handle_truncate", "stream_id": handleID, "len": 5})
	truncResp := mustReadWS(t, conn)
	if ok, _ := truncResp["ok"].(bool); !ok {
		t.Fatalf("handle_truncate response ok = false: %v", truncResp)
	}

	// Read back truncated content
	mustWriteWS(t, conn, map[string]any{"event": "handle_read", "stream_id": handleID, "len": 0, "perm": 0})
	shortResp, shortPayload := mustReadWSFrame(t, conn)
	if ok, _ := shortResp["ok"].(bool); !ok {
		t.Fatalf("handle_read after truncate response ok = false: %v", shortResp)
	}
	if string(shortPayload) != content[:5] {
		t.Fatalf("handle_read after truncate = %q, want %q", string(shortPayload), content[:5])
	}

	// Write to handle (at position 0 via Seek handled by server when len=0)
	writePayload := []byte("CHANGED")
	mustWriteWSFrame(t, conn, map[string]any{"event": "handle_write", "stream_id": handleID, "len": 0}, writePayload)
	writeResp := mustReadWS(t, conn)
	if ok, _ := writeResp["ok"].(bool); !ok {
		t.Fatalf("handle_write response ok = false: %v", writeResp)
	}

	// Sync handle
	mustWriteWS(t, conn, map[string]any{"event": "handle_sync", "stream_id": handleID})
	syncResp := mustReadWS(t, conn)
	if ok, _ := syncResp["ok"].(bool); !ok {
		t.Fatalf("handle_sync response ok = false: %v", syncResp)
	}

	// Close handle
	mustWriteWS(t, conn, map[string]any{"event": "handle_close", "stream_id": handleID})
	closeResp := mustReadWS(t, conn)
	if ok, _ := closeResp["ok"].(bool); !ok {
		t.Fatalf("handle_close response ok = false: %v", closeResp)
	}

	// Read from closed handle should error
	mustWriteWS(t, conn, map[string]any{"event": "handle_read", "stream_id": handleID, "len": 0, "perm": 0})
	closedResp := mustReadWS(t, conn)
	if ok, _ := closedResp["ok"].(bool); ok {
		t.Fatal("handle_read on closed handle returned ok=true, want error")
	}

	// Verify file on disk
	diskData, err := os.ReadFile(filepath.Join(root, "handle_test.txt"))
	if err != nil {
		t.Fatalf("ReadFile on disk error = %v", err)
	}
	// After truncate to 5 ("hello"), then write "CHANGED" at cursor position 5,
	// the file should be "helloCHANGED" (Seek(0) fix reads from 0 always)
	if string(diskData) != "helloCHANGED" {
		t.Fatalf("Final file on disk = %q, want %q", string(diskData), "helloCHANGED")
	}
}

func TestHandleChmodUtimesWire(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	content := "perm test"
	if err := os.WriteFile(filepath.Join(root, "perm_test.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Open handle
	mustWriteWS(t, conn, map[string]any{"event": "fs_open", "id": "open-chmod", "path": "perm_test.txt", "flag": "r+"})
	openResp := mustReadWS(t, conn)
	handleID, _ := openResp["stream_id"].(string)
	if handleID == "" {
		handleID, _ = openResp["handle_id"].(string)
	}
	if handleID == "" {
		t.Fatal("open response missing stream_id")
	}

	// Chmod
	mustWriteWS(t, conn, map[string]any{"event": "handle_chmod", "stream_id": handleID, "perm": 0o644})
	chmodResp := mustReadWS(t, conn)
	if ok, _ := chmodResp["ok"].(bool); !ok {
		t.Fatalf("handle_chmod response ok = false: %v", chmodResp)
	}

	// Utimes
	mustWriteWS(t, conn, map[string]any{"event": "handle_utimes", "stream_id": handleID, "atime": "2024-01-01T00:00:00Z", "mtime": "2024-06-15T12:30:00Z"})
	utimesResp := mustReadWS(t, conn)
	if ok, _ := utimesResp["ok"].(bool); !ok {
		t.Fatalf("handle_utimes response ok = false: %v", utimesResp)
	}

	// Close
	mustWriteWS(t, conn, map[string]any{"event": "handle_close", "stream_id": handleID})
	closeResp := mustReadWS(t, conn)
	if ok, _ := closeResp["ok"].(bool); !ok {
		t.Fatalf("handle_close response ok = false: %v", closeResp)
	}

	// Verify mtime on disk
	info, err := os.Stat(filepath.Join(root, "perm_test.txt"))
	if err != nil {
		t.Fatalf("Stat on disk error = %v", err)
	}
	if info.ModTime().Year() != 2024 || info.ModTime().Month() != 6 || info.ModTime().Day() != 15 {
		t.Fatalf("mtime = %v, want 2024-06-15", info.ModTime())
	}
}

func TestHandleInvalidStreamID(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	// All handle operations with a non-existent stream_id should error
	invalidID := "handle-nonexistent"

	tests := []struct {
		name  string
		event string
		extra map[string]any
	}{
		{"read", "handle_read", map[string]any{"len": 0, "perm": 0}},
		{"write", "handle_write", map[string]any{"len": 0}},
		{"close", "handle_close", nil},
		{"stat", "handle_stat", nil},
		{"truncate", "handle_truncate", map[string]any{"len": 0}},
		{"sync", "handle_sync", nil},
		{"chmod", "handle_chmod", map[string]any{"perm": 0o644}},
		{"utimes", "handle_utimes", map[string]any{"atime": "2024-01-01T00:00:00Z", "mtime": "2024-06-15T12:30:00Z"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := map[string]any{"event": tc.event, "stream_id": invalidID}
			for k, v := range tc.extra {
				msg[k] = v
			}
			if tc.event == "handle_write" {
				mustWriteWSFrame(t, conn, msg, []byte("data"))
			} else {
				mustWriteWS(t, conn, msg)
			}
			// Read next response — should be an error
			resp := mustReadWS(t, conn)
			if ok, _ := resp["ok"].(bool); ok {
				t.Fatalf("%s with invalid stream_id returned ok=true, want error", tc.event)
			}
		})
	}
}

func TestHandleReadWithOffsetAndLength(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	content := "0123456789ABCDEF"
	if err := os.WriteFile(filepath.Join(root, "offset_test.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	mustWriteWS(t, conn, map[string]any{"event": "fs_open", "id": "open-offset", "path": "offset_test.txt", "flag": "r"})
	openResp := mustReadWS(t, conn)
	handleID, _ := openResp["stream_id"].(string)
	if handleID == "" {
		handleID, _ = openResp["handle_id"].(string)
	}
	if handleID == "" {
		t.Fatal("open response missing stream_id")
	}

	// ReadAt(position=5, length=4) → expect "5678"
	mustWriteWS(t, conn, map[string]any{"event": "handle_read", "stream_id": handleID, "len": 5, "perm": 4})
	resp, payload := mustReadWSFrame(t, conn)
	if resp["ok"] != true {
		t.Fatalf("handle_read with offset failed: %v", resp)
	}
	if string(payload) != "5678" {
		t.Fatalf("handle_read(offset=5, len=4) = %q, want %q", string(payload), "5678")
	}
}

func mustWriteWSFrame(t *testing.T, conn *fakeWebSocketConn, header map[string]any, payload []byte) {
	t.Helper()
	data, err := encodeFrame(header, payload)
	if err != nil {
		t.Fatalf("encodeFrame() error = %v", err)
	}
	conn.reads <- fakeWSFrame{msgType: websocket.BinaryMessage, data: data}
}

func mustReadWSFrame(t *testing.T, conn *fakeWebSocketConn) (map[string]any, []byte) {
	t.Helper()
	select {
	case frame := <-conn.writes:
		if frame.msgType != websocket.BinaryMessage {
			t.Fatalf("message type = %d, want binary", frame.msgType)
		}
		var header map[string]any
		payload, err := decodeFrame(frame.data, &header)
		if err != nil {
			t.Fatalf("decodeFrame() error = %v; data=%s", err, string(frame.data))
		}
		return header, payload
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket frame")
	}
	return nil, nil
}
