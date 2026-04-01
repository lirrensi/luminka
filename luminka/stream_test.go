// FILE: luminka/stream_test.go
// PURPOSE: Verify stream registration, sequencing, and cleanup behavior.
// OWNS: Unit coverage for stream IDs, client chunk order, and lifecycle removal.
// EXPORTS: TestStreamRegistryRegistrationAndRemoval
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"io"
	"testing"
)

func TestStreamRegistryRegistrationAndRemoval(t *testing.T) {
	reg := newStreamRegistry()
	conn := &wsConnection{conn: &noopWebSocketConn{}}

	read := reg.registerRead(conn)
	write := reg.registerWrite(conn)
	process := reg.registerProcessOutput(conn)

	if read == nil || write == nil || process == nil {
		t.Fatal("register returned nil stream")
	}
	if read.id != "stream-1" || write.id != "stream-2" || process.id != "stream-3" {
		t.Fatalf("stream ids = %q, %q, %q; want stream-1, stream-2, stream-3", read.id, write.id, process.id)
	}
	if reg.count() != 3 {
		t.Fatalf("stream count = %d, want 3", reg.count())
	}

	reg.remove(write.id)
	if reg.count() != 2 {
		t.Fatalf("stream count after remove = %d, want 2", reg.count())
	}
	if _, ok := reg.lookup(write.id); ok {
		t.Fatal("removed stream still present")
	}
}

func TestStreamRegistryRejectsOutOfOrderClientChunkSequenceNumbers(t *testing.T) {
	state := newStreamRegistry().registerWrite(&wsConnection{conn: &noopWebSocketConn{}})
	if state == nil {
		t.Fatal("registerWrite() returned nil")
	}

	if err := state.acceptClientChunk(0); err != nil {
		t.Fatalf("acceptClientChunk(0) error = %v, want nil", err)
	}
	if err := state.acceptClientChunk(2); err == nil {
		t.Fatal("acceptClientChunk(2) error = nil, want out-of-order failure")
	}
	if err := state.acceptClientChunk(1); err != nil {
		t.Fatalf("acceptClientChunk(1) error = %v, want nil after retry", err)
	}
}

func TestStreamRegistryConnectionCleanupRemovesOwnedStreams(t *testing.T) {
	rt := &Runtime{streams: newStreamRegistry(), connections: make(map[*wsConnection]struct{})}
	conn := rt.registerConnection(&noopWebSocketConn{})
	other := rt.registerConnection(&noopWebSocketConn{})

	read := rt.streams.registerRead(conn)
	write := rt.streams.registerWrite(conn)
	process := rt.streams.registerProcessOutput(other)
	if rt.streams.count() != 3 {
		t.Fatalf("stream count = %d, want 3", rt.streams.count())
	}

	rt.unregisterConnection(conn)
	if rt.streams.count() != 1 {
		t.Fatalf("stream count after connection cleanup = %d, want 1", rt.streams.count())
	}
	if _, ok := rt.streams.lookup(read.id); ok {
		t.Fatal("read stream still present after connection cleanup")
	}
	if _, ok := rt.streams.lookup(write.id); ok {
		t.Fatal("write stream still present after connection cleanup")
	}
	if _, ok := rt.streams.lookup(process.id); !ok {
		t.Fatal("other connection stream removed unexpectedly")
	}
}

func TestRuntimeCleanupRemovesAllStreams(t *testing.T) {
	rt := &Runtime{
		streams:     newStreamRegistry(),
		connections: make(map[*wsConnection]struct{}),
		shutdownCh:  make(chan struct{}),
	}
	conn := rt.registerConnection(&noopWebSocketConn{})
	rt.streams.registerRead(conn)
	rt.streams.registerWrite(conn)

	if err := rt.cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if rt.streams.count() != 0 {
		t.Fatalf("stream count after cleanup = %d, want 0", rt.streams.count())
	}
}

type noopWebSocketConn struct{}

func (n *noopWebSocketConn) ReadMessage() (int, []byte, error) { return 0, nil, io.EOF }
func (n *noopWebSocketConn) WriteMessage(int, []byte) error    { return nil }
func (n *noopWebSocketConn) Close() error                      { return nil }
