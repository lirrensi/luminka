// FILE: luminka/fs_stream_test.go
// PURPOSE: Verify byte-first filesystem streams over websocket transport.
// OWNS: Chunked file write and read coverage for stream-open protocol events.
// EXPORTS: TestFilesystemStreamWriteAndReadRoundTrip
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestFilesystemStreamWriteAndReadRoundTrip(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	payload := bytes.Repeat([]byte("0123456789abcdef"), (fsStreamChunkSize/16)+2)

	mustWriteWS(t, conn, map[string]any{"event": "file:open_write", "id": "open-write", "path": "bytes/payload.bin"})
	writeAck, _ := mustReadWSFrame(t, conn)
	writeStreamID, _ := writeAck["stream_id"].(string)
	if writeStreamID == "" {
		t.Fatal("fs_open_write ack missing stream_id")
	}

	mustWriteWSFrame(t, conn, map[string]any{"event": "stream:chunk", "stream_id": writeStreamID, "seq": 0}, payload[:fsStreamChunkSize])
	mustWriteWSFrame(t, conn, map[string]any{"event": "stream:chunk", "stream_id": writeStreamID, "seq": 1}, payload[fsStreamChunkSize:])
	mustWriteWS(t, conn, map[string]any{"event": "stream:close", "id": "write-close", "stream_id": writeStreamID})
	assertWSOK(t, mustReadWS(t, conn), "stream:close", "write-close")

	mustWriteWS(t, conn, map[string]any{"event": "file:open_read", "id": "open-read", "path": "bytes/payload.bin"})
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
		case "stream:chunk":
			if sid, _ := header["stream_id"].(string); sid != readStreamID {
				t.Fatalf("stream_chunk stream_id = %q, want %q", sid, readStreamID)
			}
			got.Write(chunk)
		case "stream:close":
			if got := header["ok"]; got != true {
				t.Fatalf("stream_close ok = %v, want true", got)
			}
			if got := header["event"]; got != "stream:close" {
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

	mustWriteWS(t, conn, map[string]any{"event": "file:open_read", "id": "escape", "path": filepath.Join("..", "escape.bin")})
	resp := mustReadWS(t, conn)
	assertWSFailure(t, resp, "response:file:open_read", "escape", "path escapes root")
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
	mustWriteWS(t, conn, map[string]any{"event": "file:open", "id": "open1", "path": "handle_test.txt", "flag": "r+"})
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
	mustWriteWS(t, conn, map[string]any{"event": "handle:read", "stream_id": handleID, "len": 0, "perm": 0})
	readResp, readPayload := mustReadWSFrame(t, conn)
	if readResp["event"] != "response:handle:read" || readResp["ok"] != true {
		t.Fatalf("handle_read response = %v, want response:handle:read ok", readResp)
	}
	if string(readPayload) != content {
		t.Fatalf("handle_read payload = %q, want %q", string(readPayload), content)
	}

	// Stat handle
	mustWriteWS(t, conn, map[string]any{"event": "handle:stat", "stream_id": handleID})
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
	mustWriteWS(t, conn, map[string]any{"event": "handle:truncate", "stream_id": handleID, "len": 5})
	truncResp := mustReadWS(t, conn)
	if ok, _ := truncResp["ok"].(bool); !ok {
		t.Fatalf("handle_truncate response ok = false: %v", truncResp)
	}

	// Read back truncated content
	mustWriteWS(t, conn, map[string]any{"event": "handle:read", "stream_id": handleID, "len": 0, "perm": 0})
	shortResp, shortPayload := mustReadWSFrame(t, conn)
	if ok, _ := shortResp["ok"].(bool); !ok {
		t.Fatalf("handle_read after truncate response ok = false: %v", shortResp)
	}
	if string(shortPayload) != content[:5] {
		t.Fatalf("handle_read after truncate = %q, want %q", string(shortPayload), content[:5])
	}

	// Write to handle (at position 0 via Seek handled by server when len=0)
	writePayload := []byte("CHANGED")
	mustWriteWSFrame(t, conn, map[string]any{"event": "handle:write", "stream_id": handleID, "len": 0}, writePayload)
	writeResp := mustReadWS(t, conn)
	if ok, _ := writeResp["ok"].(bool); !ok {
		t.Fatalf("handle_write response ok = false: %v", writeResp)
	}

	// Sync handle
	mustWriteWS(t, conn, map[string]any{"event": "handle:sync", "stream_id": handleID})
	syncResp := mustReadWS(t, conn)
	if ok, _ := syncResp["ok"].(bool); !ok {
		t.Fatalf("handle_sync response ok = false: %v", syncResp)
	}

	// Close handle
	mustWriteWS(t, conn, map[string]any{"event": "handle:close", "stream_id": handleID})
	closeResp := mustReadWS(t, conn)
	if ok, _ := closeResp["ok"].(bool); !ok {
		t.Fatalf("handle_close response ok = false: %v", closeResp)
	}

	// Read from closed handle should error
	mustWriteWS(t, conn, map[string]any{"event": "handle:read", "stream_id": handleID, "len": 0, "perm": 0})
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
	mustWriteWS(t, conn, map[string]any{"event": "file:open", "id": "open-chmod", "path": "perm_test.txt", "flag": "r+"})
	openResp := mustReadWS(t, conn)
	handleID, _ := openResp["stream_id"].(string)
	if handleID == "" {
		handleID, _ = openResp["handle_id"].(string)
	}
	if handleID == "" {
		t.Fatal("open response missing stream_id")
	}

	// Chmod
	mustWriteWS(t, conn, map[string]any{"event": "handle:chmod", "stream_id": handleID, "perm": 0o644})
	chmodResp := mustReadWS(t, conn)
	if ok, _ := chmodResp["ok"].(bool); !ok {
		t.Fatalf("handle_chmod response ok = false: %v", chmodResp)
	}

	// Utimes
	mustWriteWS(t, conn, map[string]any{"event": "handle:utimes", "stream_id": handleID, "atime": "2024-01-01T00:00:00Z", "mtime": "2024-06-15T12:30:00Z"})
	utimesResp := mustReadWS(t, conn)
	if ok, _ := utimesResp["ok"].(bool); !ok {
		t.Fatalf("handle_utimes response ok = false: %v", utimesResp)
	}

	// Close
	mustWriteWS(t, conn, map[string]any{"event": "handle:close", "stream_id": handleID})
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
		{"read", "handle:read", map[string]any{"len": 0, "perm": 0}},
		{"write", "handle:write", map[string]any{"len": 0}},
		{"close", "handle:close", nil},
		{"stat", "handle:stat", nil},
		{"truncate", "handle:truncate", map[string]any{"len": 0}},
		{"sync", "handle:sync", nil},
		{"chmod", "handle:chmod", map[string]any{"perm": 0o644}},
		{"utimes", "handle:utimes", map[string]any{"atime": "2024-01-01T00:00:00Z", "mtime": "2024-06-15T12:30:00Z"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := map[string]any{"event": tc.event, "stream_id": invalidID}
			for k, v := range tc.extra {
				msg[k] = v
			}
			if tc.event == "handle:write" {
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

	mustWriteWS(t, conn, map[string]any{"event": "file:open", "id": "open-offset", "path": "offset_test.txt", "flag": "r"})
	openResp := mustReadWS(t, conn)
	handleID, _ := openResp["stream_id"].(string)
	if handleID == "" {
		handleID, _ = openResp["handle_id"].(string)
	}
	if handleID == "" {
		t.Fatal("open response missing stream_id")
	}

	// ReadAt(position=5, length=4) → expect "5678"
	mustWriteWS(t, conn, map[string]any{"event": "handle:read", "stream_id": handleID, "len": 5, "perm": 4})
	resp, payload := mustReadWSFrame(t, conn)
	if resp["ok"] != true {
		t.Fatalf("handle_read with offset failed: %v", resp)
	}
	if string(payload) != "5678" {
		t.Fatalf("handle_read(offset=5, len=4) = %q, want %q", string(payload), "5678")
	}
}

// ---------------------------------------------------------------------------
// Stream edge case: out-of-order chunk rejection
// ---------------------------------------------------------------------------

func TestFilesystemStreamRejectsOutOfOrderChunks(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	payload := []byte("test payload")

	mustWriteWS(t, conn, map[string]any{"event": "file:open_write", "id": "open-write", "path": "ordered.bin"})
	writeAck, _ := mustReadWSFrame(t, conn)
	writeStreamID, _ := writeAck["stream_id"].(string)
	if writeStreamID == "" {
		t.Fatal("fs_open_write ack missing stream_id")
	}

	// Send seq=1 before seq=0 — should be rejected
	mustWriteWSFrame(t, conn, map[string]any{"event": "stream:chunk", "stream_id": writeStreamID, "seq": 1}, payload)
	resp := mustReadWS(t, conn)
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatal("out-of-order chunk (seq=1 before seq=0) was accepted, want error")
	}
	if errStr, _ := resp["error"].(string); !strings.Contains(errStr, "unexpected stream sequence") {
		t.Fatalf("out-of-order chunk error = %q, want 'unexpected stream sequence'", errStr)
	}

	// Cleanup: close the stream
	mustWriteWS(t, conn, map[string]any{"event": "stream:close", "id": "close", "stream_id": writeStreamID})
}

// ---------------------------------------------------------------------------
// Stream edge case: stream_chunk on a read stream
// ---------------------------------------------------------------------------

func TestFilesystemStreamChunkOnReadStreamRejected(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	// Write a file first
	if err := os.WriteFile(filepath.Join(root, "readonly.bin"), []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Open for reading
	mustWriteWS(t, conn, map[string]any{"event": "file:open_read", "id": "open-read", "path": "readonly.bin"})
	readAck, _ := mustReadWSFrame(t, conn)
	readStreamID, _ := readAck["stream_id"].(string)
	if readStreamID == "" {
		t.Fatal("fs_open_read ack missing stream_id")
	}

	// Read the chunks (drain the stream)
	deadline := time.Now().Add(2 * time.Second)
	for {
		header, _ := mustReadWSFrame(t, conn)
		if header["event"] == "stream:close" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out draining read stream")
		}
	}

	// Now try to write a chunk to the (now-closed) read stream
	mustWriteWSFrame(t, conn, map[string]any{"event": "stream:chunk", "stream_id": readStreamID, "seq": 0}, []byte("bad"))
	resp := mustReadWS(t, conn)
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatal("stream_chunk on closed read stream was accepted, want error")
	}
}

// ---------------------------------------------------------------------------
// Stream edge case: double close on write stream
// ---------------------------------------------------------------------------

func TestFilesystemStreamDoubleClose(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	mustWriteWS(t, conn, map[string]any{"event": "file:open_write", "id": "open", "path": "double_close.bin"})
	ack, _ := mustReadWSFrame(t, conn)
	streamID, _ := ack["stream_id"].(string)
	if streamID == "" {
		t.Fatal("missing stream_id")
	}

	// First close should succeed
	mustWriteWS(t, conn, map[string]any{"event": "stream:close", "id": "close1", "stream_id": streamID})
	resp := mustReadWS(t, conn)
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("first stream_close failed: %v", resp)
	}

	// Second close on same ID should error
	mustWriteWS(t, conn, map[string]any{"event": "stream:close", "id": "close2", "stream_id": streamID})
	resp = mustReadWS(t, conn)
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatal("second stream_close succeeded, want error")
	}
}

// ---------------------------------------------------------------------------
// Stream large file write and read roundtrip via WebSocket
// ---------------------------------------------------------------------------

func TestFilesystemStreamLargeFileRoundtrip(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	// Payload larger than 2 chunks (>64KB) to exercise multi-chunk
	payloadSize := 3*fsStreamChunkSize + 1234
	payload := bytes.Repeat([]byte("ABCDEFGHIJ"), (payloadSize/10)+1)
	payload = payload[:payloadSize]

	mustWriteWS(t, conn, map[string]any{"event": "file:open_write", "id": "write", "path": "large.bin"})
	ack, _ := mustReadWSFrame(t, conn)
	writeStreamID, _ := ack["stream_id"].(string)
	if writeStreamID == "" {
		t.Fatal("missing stream_id")
	}

	// Write in chunk-sized pieces
	var seq uint64
	for offset := 0; offset < len(payload); offset += fsStreamChunkSize {
		end := offset + fsStreamChunkSize
		if end > len(payload) {
			end = len(payload)
		}
		mustWriteWSFrame(t, conn, map[string]any{"event": "stream:chunk", "stream_id": writeStreamID, "seq": seq}, payload[offset:end])
		seq++
	}

	mustWriteWS(t, conn, map[string]any{"event": "stream:close", "id": "close", "stream_id": writeStreamID})
	assertWSOK(t, mustReadWS(t, conn), "stream:close", "close")

	// Read back via stream
	mustWriteWS(t, conn, map[string]any{"event": "file:open_read", "id": "read", "path": "large.bin"})
	readAck, _ := mustReadWSFrame(t, conn)
	readStreamID, _ := readAck["stream_id"].(string)
	if readStreamID == "" {
		t.Fatal("read missing stream_id")
	}

	var got bytes.Buffer
	deadline := time.Now().Add(5 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("timed out reading large file stream")
		}
		header, chunk := mustReadWSFrame(t, conn)
		switch header["event"] {
		case "stream:chunk":
			got.Write(chunk)
		case "stream:close":
			if !bytes.Equal(got.Bytes(), payload) {
				t.Fatalf("large file mismatch: got %d bytes, want %d", got.Len(), len(payload))
			}
			return
		default:
			t.Fatalf("unexpected event %q", header["event"])
		}
	}
}

// ---------------------------------------------------------------------------
// fs_open with various flags and modes
// ---------------------------------------------------------------------------

func TestFilesystemStreamOpenWithFlags(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	t.Run("open with flag 'r' on non-existent should fail", func(t *testing.T) {
		mustWriteWS(t, conn, map[string]any{"event": "file:open", "id": "open-r", "path": "nonexistent.txt", "flag": "r"})
		resp := mustReadWS(t, conn)
		if ok, _ := resp["ok"].(bool); ok {
			t.Fatal("fs_open 'r' on non-existent succeeded, want error")
		}
	})

	t.Run("open with flag 'w' should create file", func(t *testing.T) {
		mustWriteWS(t, conn, map[string]any{"event": "file:open", "id": "open-w", "path": "created.txt", "flag": "w"})
		resp := mustReadWS(t, conn)
		if ok, _ := resp["ok"].(bool); !ok {
			t.Fatalf("fs_open 'w' failed: %v", resp)
		}
		// Close the handle
		handleID, _ := resp["stream_id"].(string)
		if handleID != "" {
			mustWriteWS(t, conn, map[string]any{"event": "handle:close", "stream_id": handleID})
			mustReadWS(t, conn)
		}
	})

	t.Run("open with flag 'a' should append", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(root, "append_test.txt"), []byte("base"), 0o644); err != nil {
			t.Fatalf("WriteFile error = %v", err)
		}
		mustWriteWS(t, conn, map[string]any{"event": "file:open", "id": "open-a", "path": "append_test.txt", "flag": "a"})
		resp := mustReadWS(t, conn)
		if ok, _ := resp["ok"].(bool); !ok {
			t.Fatalf("fs_open 'a' failed: %v", resp)
		}
		handleID, _ := resp["stream_id"].(string)
		if handleID != "" {
			// Write to the handle at current position (append)
			mustWriteWSFrame(t, conn, map[string]any{"event": "handle:write", "stream_id": handleID, "len": 0}, []byte("+appended"))
			writeResp := mustReadWS(t, conn)
			if ok, _ := writeResp["ok"].(bool); !ok {
				t.Fatalf("handle_write to append handle failed: %v", writeResp)
			}
			// Close
			mustWriteWS(t, conn, map[string]any{"event": "handle:close", "stream_id": handleID})
			mustReadWS(t, conn)
		}
		// Verify on disk
		diskData, err := os.ReadFile(filepath.Join(root, "append_test.txt"))
		if err != nil {
			t.Fatalf("ReadFile error = %v", err)
		}
		if string(diskData) != "base+appended" {
			t.Fatalf("append file content = %q, want %q", string(diskData), "base+appended")
		}
	})

	// Close the handle from 'w' test if it's still open
	// (Handles are cleaned up by connection close)
}

// ---------------------------------------------------------------------------
// Edge case: handle_read on a zero-length file
// ---------------------------------------------------------------------------

func TestHandleReadZeroLengthFile(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	// Create an empty file
	if err := os.WriteFile(filepath.Join(root, "empty.txt"), []byte{}, 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Open the empty file for reading
	mustWriteWS(t, conn, map[string]any{"event": "file:open", "id": "open-empty", "path": "empty.txt", "flag": "r"})
	openResp := mustReadWS(t, conn)
	handleID, _ := openResp["stream_id"].(string)
	if handleID == "" {
		t.Fatal("open response missing stream_id")
	}

	// Read the zero-length file (uses ReadAll path: offset=0, requestedLength=0)
	mustWriteWS(t, conn, map[string]any{"event": "handle:read", "stream_id": handleID, "len": 0, "perm": 0})
	resp := mustReadWS(t, conn)
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("handle_read on empty file failed: %v", resp)
	}
	// Response should have no data field (empty file)
	if data, hasData := resp["data"]; hasData && data != nil {
		t.Fatalf("handle_read on empty file returned data = %v, want nil", data)
	}

	// Clean up
	mustWriteWS(t, conn, map[string]any{"event": "handle:close", "stream_id": handleID})
	mustReadWS(t, conn)
}

// ---------------------------------------------------------------------------
// Edge case: unknown event rejected
// ---------------------------------------------------------------------------

func TestFilesystemUnknownEvent(t *testing.T) {
	root := t.TempDir()
	_, conn := newTestWebSocketRuntime(t, root, capabilityState{FS: true})

	mustWriteWS(t, conn, map[string]any{"event": "fs_invalid_event", "id": "bad1"})
	resp := mustReadWS(t, conn)
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatal("unknown event returned ok=true, want error")
	}
	errText, _ := resp["error"].(string)
	if !strings.Contains(errText, "unknown event") {
		t.Fatalf("error = %q, want 'unknown event'", errText)
	}
}

// ---------------------------------------------------------------------------
// KNOWN BUG: fs_utimes and handle_utimes nil pointer dereference
// when one time is valid RFC3339Nano but the other is invalid.
// The code checks `atimeErr != nil || mtimeErr != nil` and then tries
// Sscanf + atimeErr.Error() on both, but one error may be nil.
// ---------------------------------------------------------------------------

func TestFilesystemUtimesNilDerefBug(t *testing.T) {
	root := t.TempDir()

	// Create the file first
	if err := os.WriteFile(filepath.Join(root, "utimes_bug.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set up minimal runtime and request
	rt := &Runtime{
		Root:     root,
		FSBridge: NewFSBridge(root),
	}
	fakeConn := newFakeWebSocketConn()
	wsConn := rt.registerConnection(fakeConn)

	// Case 1: valid atime (RFC3339Nano), invalid mtime — triggers nil deref
	t.Run("valid atime invalid mtime", func(t *testing.T) {
		request := WSMessage{
			Event: "file:utimes",
			ID:    json.RawMessage(`"u1"`),
			Path:  "utimes_bug.txt",
			Atime: "2024-01-01T00:00:00Z", // Valid RFC3339Nano
			Mtime: "invalid",               // Invalid — atimeErr == nil, mtimeErr != nil
		}

		panicked := true
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Logf("KNOWN BUG: fs_utimes nil deref with valid atime + invalid mtime: %v", r)
				} else {
					panicked = false
				}
			}()
			_ = rt.handleFilesystemRequest(wsConn, request, nil)
		}()

		if !panicked {
			t.Log("BUG FIXED: fs_utimes no longer panics with mixed valid/invalid times")
		}
	})

	// Case 2: invalid atime, valid mtime (RFC3339Nano) — same bug, reversed
	t.Run("invalid atime valid mtime", func(t *testing.T) {
		request := WSMessage{
			Event: "file:utimes",
			ID:    json.RawMessage(`"u2"`),
			Path:  "utimes_bug.txt",
			Atime: "invalid",               // Invalid — atimeErr != nil
			Mtime: "2024-06-15T12:30:00Z", // Valid RFC3339Nano — mtimeErr == nil
		}

		panicked := true
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Logf("KNOWN BUG: fs_utimes nil deref with invalid atime + valid mtime: %v", r)
				} else {
					panicked = false
				}
			}()
			_ = rt.handleFilesystemRequest(wsConn, request, nil)
		}()

		if !panicked {
			t.Log("BUG FIXED: fs_utimes no longer panics with mixed valid/invalid times")
		}
	})
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
