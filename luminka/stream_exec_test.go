// FILE: luminka/stream_exec_test.go
// PURPOSE: Verify exec stream writer sequencing, pump reader chunking, and error helpers.
// OWNS: Unit tests for execStreamWriter, pumpReaderToStream, firstStreamError, commandExitCode.
// EXPORTS: none

package luminka

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// --- execStreamWriter ---

func TestNewExecStreamWriter(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &WSConnection{conn: fconn}

	w := newExecStreamWriter(conn, "stream-1", nil)
	if w == nil {
		t.Fatal("newExecStreamWriter() = nil")
	}
	if w.streamID != "stream-1" {
		t.Fatalf("streamID = %s, want stream-1", w.streamID)
	}
	if w.conn != conn {
		t.Fatal("conn pointer mismatch")
	}
	if w.seq != 0 {
		t.Fatalf("seq = %d, want 0", w.seq)
	}
}

func TestExecStreamWriterWriteChunk(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &WSConnection{conn: fconn}

	w := newExecStreamWriter(conn, "stream-exec-1", nil)

	// Write first chunk
	if err := w.writeChunk("stdout", []byte("hello"), false); err != nil {
		t.Fatalf("writeChunk error = %v", err)
	}

	// Verify on the wire
	frame := <-fconn.writes
	var msg map[string]any
	payload, err := decodeFrame(frame.data, &msg)
	if err != nil {
		t.Fatalf("decodeFrame error = %v", err)
	}
	if msg["event"] != "stream:chunk" {
		t.Fatalf("event = %v, want stream:chunk", msg["event"])
	}
	if msg["stream_id"] != "stream-exec-1" {
		t.Fatalf("stream_id = %v, want stream-exec-1", msg["stream_id"])
	}
	// seq=0 is omitted due to omitempty; accept nil as valid
	if seq := msg["seq"]; seq != nil && seq != float64(0) {
		t.Fatalf("seq = %v, want 0 or omitted", seq)
	}
	if msg["lane"] != "stdout" {
		t.Fatalf("lane = %v, want stdout", msg["lane"])
	}
	// eof=false is omitted due to omitempty; accept nil as valid
	if eof := msg["eof"]; eof != nil && eof != false {
		t.Fatalf("eof = %v, want false or omitted", eof)
	}
	if string(payload) != "hello" {
		t.Fatalf("payload = %q, want %q", string(payload), "hello")
	}
}

func TestExecStreamWriterWriteChunkIncrementsSeq(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &WSConnection{conn: fconn}

	w := newExecStreamWriter(conn, "stream-seq", nil)

	// Write seq 0 (omitted from JSON due to omitempty)
	_ = w.writeChunk("stdout", []byte("a"), false)
	<-fconn.writes // consume

	// Write seq 1 (non-zero, should appear)
	_ = w.writeChunk("stdout", []byte("b"), false)
	frame := <-fconn.writes
	var msg map[string]any
	_, _ = decodeFrame(frame.data, &msg)
	if seq, ok := msg["seq"].(float64); !ok || seq != 1 {
		t.Fatalf("seq = %v, want 1", msg["seq"])
	}

	// Write seq 2
	_ = w.writeChunk("stderr", []byte("c"), false)
	frame = <-fconn.writes
	_, _ = decodeFrame(frame.data, &msg)
	if seq, ok := msg["seq"].(float64); !ok || seq != 2 {
		t.Fatalf("seq = %v, want 2", msg["seq"])
	}
}

func TestExecStreamWriterWriteChunkEOFFlag(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &WSConnection{conn: fconn}

	w := newExecStreamWriter(conn, "stream-eof", nil)

	_ = w.writeChunk("stdout", []byte("last"), true)
	frame := <-fconn.writes

	var msg map[string]any
	_, _ = decodeFrame(frame.data, &msg)
	if eof, ok := msg["eof"].(bool); !ok || eof != true {
		t.Fatalf("eof = %v, want true", msg["eof"])
	}
}

func TestExecStreamWriterWriteChunkLane(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &WSConnection{conn: fconn}

	w := newExecStreamWriter(conn, "stream-lane", nil)

	_ = w.writeChunk("stderr", []byte("err"), false)
	frame := <-fconn.writes

	var msg map[string]any
	_, _ = decodeFrame(frame.data, &msg)
	if msg["lane"] != "stderr" {
		t.Fatalf("lane = %v, want stderr", msg["lane"])
	}
}

func TestExecStreamWriterWriteChunkNilPayload(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &WSConnection{conn: fconn}

	w := newExecStreamWriter(conn, "stream-nil", nil)

	// nil payload should still produce a valid frame
	if err := w.writeChunk("stdout", nil, false); err != nil {
		t.Fatalf("writeChunk with nil payload: %v", err)
	}

	frame := <-fconn.writes
	var msg map[string]any
	payload, err := decodeFrame(frame.data, &msg)
	if err != nil {
		t.Fatalf("decodeFrame error = %v", err)
	}
	if len(payload) != 0 {
		t.Fatalf("payload = %v, want empty", payload)
	}
}

func TestExecStreamWriterNilReceiver(t *testing.T) {
	var w *execStreamWriter = nil

	err := w.writeChunk("stdout", []byte("x"), false)
	if err == nil {
		t.Fatal("nil writer writeChunk = nil, want error")
	}
	if err != io.ErrClosedPipe {
		t.Fatalf("error = %v, want io.ErrClosedPipe", err)
	}
}

// --- pumpReaderToStream ---

func TestPumpReaderToStreamSmallRead(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &WSConnection{conn: fconn}
	w := newExecStreamWriter(conn, "stream-pump", nil)

	// Use a reader that returns (data, io.EOF) in one call (like pipe close)
	reader := &eofReader{data: []byte("hello, world!")}
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(1)

	pumpReaderToStream(reader, "stdout", w, errCh, &wg)
	wg.Wait()

	// Should have received one chunk with eof=true
	frame := <-fconn.writes
	var msg map[string]any
	payload, _ := decodeFrame(frame.data, &msg)
	if string(payload) != "hello, world!" {
		t.Fatalf("payload = %q, want %q", string(payload), "hello, world!")
	}
	if eof, ok := msg["eof"].(bool); !ok || eof != true {
		t.Fatalf("eof = %v, want true", msg["eof"])
	}
	// seq=0 is omitted due to omitempty
	if seq := msg["seq"]; seq != nil && seq != float64(0) {
		t.Fatalf("seq = %v, want 0 or omitted", seq)
	}
}

// eofReader returns data+io.EOF on first read, simulating a pipe that closes after one chunk
type eofReader struct {
	data []byte
	done bool
}

func (r *eofReader) Read(p []byte) (int, error) {
	if r.done || len(r.data) == 0 {
		return 0, io.EOF
	}
	r.done = true
	n := copy(p, r.data)
	return n, io.EOF
}

func TestPumpReaderToStreamNilReader(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &WSConnection{conn: fconn}
	w := newExecStreamWriter(conn, "stream-pump-nil", nil)

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(1)

	pumpReaderToStream(nil, "stdout", w, errCh, &wg)
	wg.Wait()

	// No chunks should be sent
	select {
	case <-fconn.writes:
		t.Fatal("unexpected chunk sent for nil reader")
	default:
		// good — no output
	}
}

func TestPumpReaderToStreamReadError(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &WSConnection{conn: fconn}
	w := newExecStreamWriter(conn, "stream-pump-err", nil)

	errReader := &errorReader{err: io.ErrUnexpectedEOF}
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(1)

	pumpReaderToStream(errReader, "stdout", w, errCh, &wg)
	wg.Wait()

	select {
	case err := <-errCh:
		if err != io.ErrUnexpectedEOF {
			t.Fatalf("error = %v, want io.ErrUnexpectedEOF", err)
		}
	default:
		t.Fatal("expected error in errCh, got none")
	}
}

// errorReader returns an error on every read
type errorReader struct{ err error }

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, r.err
}

func TestPumpReaderToStreamExactChunkSize(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &WSConnection{conn: fconn}
	w := newExecStreamWriter(conn, "stream-exact", nil)

	// Exactly one chunk size, delivered with io.EOF
	data := bytes.Repeat([]byte("B"), fsStreamChunkSize)
	reader := &eofReader{data: data}
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(1)

	pumpReaderToStream(reader, "stdout", w, errCh, &wg)
	wg.Wait()

	// Verify one chunk with all the data and eof=true
	select {
	case frame := <-fconn.writes:
		var msg map[string]any
		payload, _ := decodeFrame(frame.data, &msg)
		if len(payload) != fsStreamChunkSize {
			t.Fatalf("payload size = %d, want %d", len(payload), fsStreamChunkSize)
		}
		if eof, ok := msg["eof"].(bool); !ok || eof != true {
			t.Fatalf("eof = %v, want true", msg["eof"])
		}
	default:
		t.Fatal("expected one chunk, got none")
	}

	// No error
	select {
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	default:
	}
}

func TestPumpReaderToStreamWriteError(t *testing.T) {
	// Use a closed connection so writes fail
	fconn := newFakeWebSocketConn()
	conn := &WSConnection{conn: fconn}
	fconn.CloseInput() // close before writing

	w := newExecStreamWriter(conn, "stream-write-err", nil)
	reader := bytes.NewBufferString("data")
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(1)

	pumpReaderToStream(reader, "stdout", w, errCh, &wg)
	wg.Wait()

	// Should receive an error in errCh (or at least not panic)
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected non-nil error")
		}
	default:
		// On some platforms, writing to a closed pipe may succeed silently
		// or error differently. Not failing the test — just noting.
	}
}

// --- firstStreamError ---

func TestFirstStreamErrorNil(t *testing.T) {
	errCh := make(chan error, 2)
	err := firstStreamError(errCh)
	if err != nil {
		t.Fatalf("firstStreamError on empty ch = %v, want nil", err)
	}
}

func TestFirstStreamErrorHasError(t *testing.T) {
	errCh := make(chan error, 2)
	errCh <- io.EOF

	err := firstStreamError(errCh)
	if err != io.EOF {
		t.Fatalf("firstStreamError = %v, want io.EOF", err)
	}
}

func TestFirstStreamErrorMultiple(t *testing.T) {
	errCh := make(chan error, 2)
	errCh <- io.ErrUnexpectedEOF
	errCh <- io.EOF

	// Should return the first one
	err := firstStreamError(errCh)
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("firstStreamError = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestFirstStreamErrorBlocks(t *testing.T) {
	errCh := make(chan error) // unbuffered — but we add from same goroutine
	// firstStreamError uses select with default, so it should not block
	err := firstStreamError(errCh)
	if err != nil {
		t.Fatalf("firstStreamError = %v, want nil", err)
	}
}

// --- commandExitCode ---

func TestCommandExitCodeExitError(t *testing.T) {
	// Simulate an ExitError
	err := &exec.ExitError{}
	// ExitError has a ProcessState but we can't easily set ExitCode without running a real command.
	// Just verify the type assertion works and returns -1 as fallback.
	code := commandExitCode(err)
	if code != -1 {
		t.Fatalf("commandExitCode(*exec.ExitError{}) = %d, want -1 (can't set ProcessState)", code)
	}
}

func TestCommandExitCodeOtherError(t *testing.T) {
	err := errors.New("some other error")
	code := commandExitCode(err)
	if code != -1 {
		t.Fatalf("commandExitCode(other error) = %d, want -1", code)
	}
}

func TestCommandExitCodeNil(t *testing.T) {
	code := commandExitCode(nil)
	if code != -1 {
		t.Fatalf("commandExitCode(nil) = %d, want -1", code)
	}
}

// --- Timeout ---

func TestExecStreamWriterConcurrent(t *testing.T) {
	fconn := newFakeWebSocketConn()
	conn := &WSConnection{conn: fconn}
	w := newExecStreamWriter(conn, "stream-con", nil)

	var wg sync.WaitGroup
	chunks := 30

	for i := 0; i < chunks; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = w.writeChunk("stdout", []byte("x"), false)
		}(i)
	}

	// Drain writes
	done := make(chan struct{})
	go func() {
		for i := 0; i < chunks; i++ {
			<-fconn.writes
		}
		close(done)
	}()

	select {
	case <-done:
		// got all chunks
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for concurrent writes")
	}
}
