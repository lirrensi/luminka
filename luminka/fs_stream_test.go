// FILE: luminka/fs_stream_test.go
// PURPOSE: Verify byte-first filesystem streams over websocket transport.
// OWNS: Chunked file write and read coverage for stream-open protocol events.
// EXPORTS: TestFilesystemStreamWriteAndReadRoundTrip
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"bytes"
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
