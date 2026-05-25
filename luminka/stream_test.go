// FILE: luminka/stream_test.go
// PURPOSE: Verify stream registry lifecycle, sequence validation, nil safety, and concurrent access.
// OWNS: Unit tests for newStreamRegistry → register/lookup/remove/close lifecycle and streamState.
// EXPORTS: none

package luminka

import (
	"os"
	"sync"
	"testing"
)

// --- StreamRegistry: Construction and Count ---

func TestNewStreamRegistry(t *testing.T) {
	sr := newStreamRegistry()
	if sr == nil {
		t.Fatal("newStreamRegistry() = nil")
	}
	if c := sr.count(); c != 0 {
		t.Fatalf("count = %d, want 0", c)
	}
}

// --- StreamRegistry: Register and Kind Assignment ---

func TestStreamRegistryRegisterRead(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	s := sr.registerRead(conn)
	if s == nil {
		t.Fatal("registerRead() = nil")
	}
	if s.kind != streamKindRead {
		t.Fatalf("kind = %v, want %v", s.kind, streamKindRead)
	}
	if s.conn != conn {
		t.Fatal("conn pointer mismatch")
	}
	if s.id == "" {
		t.Fatal("stream id is empty")
	}
	if c := sr.count(); c != 1 {
		t.Fatalf("count = %d, want 1", c)
	}
}

func TestStreamRegistryRegisterWrite(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	s := sr.registerWrite(conn)
	if s == nil {
		t.Fatal("registerWrite() = nil")
	}
	if s.kind != streamKindWrite {
		t.Fatalf("kind = %v, want %v", s.kind, streamKindWrite)
	}
}

func TestStreamRegistryRegisterProcessOutput(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	s := sr.registerProcessOutput(conn)
	if s == nil {
		t.Fatal("registerProcessOutput() = nil")
	}
	if s.kind != streamKindProcessOutput {
		t.Fatalf("kind = %v, want %v", s.kind, streamKindProcessOutput)
	}
}

func TestStreamRegistryRegisterHandle(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	// nil file → nil state
	s := sr.registerHandle(conn, nil)
	if s != nil {
		t.Fatal("registerHandle with nil file should return nil")
	}

	// nil conn → nil state
	s = sr.registerHandle(nil, &os.File{})
	if s != nil {
		t.Fatal("registerHandle with nil conn should return nil")
	}

	// valid args → valid handle stream
	f, err := os.CreateTemp("", "stream-test-handle-*")
	if err != nil {
		t.Fatal(err)
	}
	// stream owns the file now — it will be closed via remove
	s = sr.registerHandle(conn, f)
	if s == nil {
		t.Fatal("registerHandle with valid args = nil")
	}
	if s.kind != streamKindHandle {
		t.Fatalf("kind = %v, want %v", s.kind, streamKindHandle)
	}
	if s.file != f {
		t.Fatal("file pointer mismatch")
	}

	// clean up closes the file
	sr.remove(s.id)
	// verify file is closed (write should fail on closed file)
	var buf [1]byte
	if _, err := f.Write(buf[:]); err == nil {
		t.Fatal("expected write error on closed file")
	}
}

func TestStreamRegistryRegisterIDUniqueness(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	s1 := sr.registerRead(conn)
	s2 := sr.registerRead(conn)
	s3 := sr.registerWrite(conn)

	if s1.id == s2.id {
		t.Fatal("s1 and s2 have same id")
	}
	if s2.id == s3.id {
		t.Fatal("s2 and s3 have same id")
	}
}

// --- StreamRegistry: Lookup ---

func TestStreamRegistryLookup(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	s := sr.registerRead(conn)

	// found
	got, ok := sr.lookup(s.id)
	if !ok {
		t.Fatal("lookup should find registered stream")
	}
	if got != s {
		t.Fatal("lookup returned wrong pointer")
	}

	// not found
	_, ok = sr.lookup("non-existent-id")
	if ok {
		t.Fatal("lookup should not find unregistered id")
	}

	// empty string
	_, ok = sr.lookup("")
	if ok {
		t.Fatal("lookup should not find empty id")
	}
}

// --- StreamRegistry: Remove ---

func TestStreamRegistryRemove(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	s := sr.registerRead(conn)
	sr.remove(s.id)

	if c := sr.count(); c != 0 {
		t.Fatalf("count = %d, want 0", c)
	}
	if !s.closed {
		t.Fatal("stream should be marked closed after remove")
	}
	if _, ok := sr.lookup(s.id); ok {
		t.Fatal("lookup should not find removed stream")
	}
}

func TestStreamRegistryRemoveIdempotent(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	s := sr.registerRead(conn)
	sr.remove(s.id)
	sr.remove(s.id)  // second remove — should not panic
	sr.remove("")     // empty id — should not panic
	sr.remove("non-existent")  // non-existent — should not panic
}

func TestStreamRegistryRemoveCleansByConnection(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	s1 := sr.registerRead(conn)
	_ = sr.registerWrite(conn)
	sr.remove(s1.id)

	// conn should still appear in byConnection (second stream is there)
	ids := sr.closeConnection(conn)
	if len(ids) != 1 {
		t.Fatalf("closeConnection after partial remove returned %d ids, want 1", len(ids))
	}
}

// --- StreamRegistry: CloseConnection ---

func TestStreamRegistryCloseConnection(t *testing.T) {
	sr := newStreamRegistry()
	conn1 := &wsConnection{}
	conn2 := &wsConnection{}

	s1 := sr.registerRead(conn1)
	s2 := sr.registerWrite(conn1)
	s3 := sr.registerProcessOutput(conn2)

	ids := sr.closeConnection(conn1)
	if len(ids) != 2 {
		t.Fatalf("closeConnection returned %d ids, want 2", len(ids))
	}
	if !s1.closed || !s2.closed {
		t.Fatal("conn1 streams should be closed")
	}
	if s3.closed {
		t.Fatal("conn2 stream should NOT be closed")
	}
	if c := sr.count(); c != 1 {
		t.Fatalf("count = %d, want 1 (conn2 stream remains)", c)
	}

	// clean up remaining
	sr.closeConnection(conn2)
	if c := sr.count(); c != 0 {
		t.Fatalf("count = %d, want 0", c)
	}

	// closeConnection on connection with no streams
	sr.closeConnection(&wsConnection{})
}

func TestStreamRegistryCloseConnectionByConnectionCleanup(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	sr.registerRead(conn)
	sr.registerWrite(conn)
	sr.closeConnection(conn)

	// byConnection map entry should be deleted
	if _, ok := sr.byConnection[conn]; ok {
		t.Fatal("byConnection entry should be deleted after closeConnection")
	}
}

// --- StreamRegistry: CloseAll ---

func TestStreamRegistryCloseAll(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	sr.registerRead(conn)
	sr.registerWrite(conn)
	sr.registerProcessOutput(conn)

	ids := sr.closeAll()
	if len(ids) != 3 {
		t.Fatalf("closeAll returned %d ids, want 3", len(ids))
	}
	if c := sr.count(); c != 0 {
		t.Fatalf("count = %d, want 0", c)
	}

	// empty registry — no panic
	ids = sr.closeAll()
	if len(ids) != 0 {
		t.Fatalf("closeAll on empty registry returned %d ids, want 0", len(ids))
	}
}

// --- StreamRegistry: Count ---

func TestStreamRegistryCount(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	if c := sr.count(); c != 0 {
		t.Fatalf("count = %d, want 0", c)
	}

	sr.registerRead(conn)
	if c := sr.count(); c != 1 {
		t.Fatalf("count = %d, want 1", c)
	}

	sr.registerWrite(conn)
	if c := sr.count(); c != 2 {
		t.Fatalf("count = %d, want 2", c)
	}

	sr.closeAll()
	if c := sr.count(); c != 0 {
		t.Fatalf("count = %d, want 0 after closeAll", c)
	}
}

// --- StreamRegistry: Nil Receiver Safety ---

func TestStreamRegistryNilReceiver(t *testing.T) {
	var sr *streamRegistry = nil

	if s := sr.registerRead(&wsConnection{}); s != nil {
		t.Fatal("nil registry registerRead should return nil")
	}
	if s := sr.registerWrite(&wsConnection{}); s != nil {
		t.Fatal("nil registry registerWrite should return nil")
	}
	if s := sr.registerProcessOutput(&wsConnection{}); s != nil {
		t.Fatal("nil registry registerProcessOutput should return nil")
	}
	if s := sr.registerHandle(&wsConnection{}, &os.File{}); s != nil {
		t.Fatal("nil registry registerHandle should return nil")
	}
	if s, ok := sr.lookup("x"); s != nil || ok {
		t.Fatal("nil registry lookup should return nil, false")
	}
	if c := sr.count(); c != 0 {
		t.Fatal("nil registry count should return 0")
	}
	// These should not panic
	sr.remove("x")
	sr.closeConnection(&wsConnection{})
	sr.closeAll()
}

// --- streamState: Nil Receiver Safety ---

func TestStreamStateNilReceiver(t *testing.T) {
	var s *streamState = nil

	if err := s.acceptClientChunk(0); err == nil || err.Error() != "stream is required" {
		t.Fatalf("nil state acceptClientChunk = %v, want 'stream is required'", err)
	}

	// These should not panic
	s.attachFile(&os.File{})
	s.closeResource()
}

// --- streamState: attachFile and closeResource ---

func TestStreamStateAttachFile(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}
	s := sr.registerRead(conn)

	if s.file != nil {
		t.Fatal("new stream should have nil file")
	}

	f, err := os.CreateTemp("", "stream-test-attach-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close() // we just need the *os.File, it's already closed

	s.attachFile(f)
	if s.file != f {
		t.Fatal("attachFile did not set file")
	}

	// closeResource should close it
	s.closeResource() // close on already-closed file — should not panic
	if s.file != nil {
		t.Fatal("closeResource should nil the file")
	}

	// second closeResource is idempotent
	s.closeResource() // should not panic
}

func TestStreamStateCloseResourceWithNilFile(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}
	s := sr.registerRead(conn)

	s.closeResource() // should not panic — file is nil
}

// --- streamState: acceptClientChunk ---

func TestAcceptClientChunk(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}
	s := sr.registerWrite(conn)

	// first chunk at seq 0
	if err := s.acceptClientChunk(0); err != nil {
		t.Fatalf("acceptClientChunk(0) = %v, want nil", err)
	}
	if s.nextSeq != 1 {
		t.Fatalf("nextSeq = %d, want 1", s.nextSeq)
	}

	// second chunk at seq 1
	if err := s.acceptClientChunk(1); err != nil {
		t.Fatalf("acceptClientChunk(1) = %v, want nil", err)
	}
	if s.nextSeq != 2 {
		t.Fatalf("nextSeq = %d, want 2", s.nextSeq)
	}
}

func TestAcceptClientChunkWrongSequence(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}
	s := sr.registerWrite(conn)

	// accept seq 0
	_ = s.acceptClientChunk(0)

	// send seq 0 again instead of 1
	err := s.acceptClientChunk(0)
	if err == nil {
		t.Fatal("acceptClientChunk(0) after seq 0 should error")
	}
	if err.Error() != "unexpected stream sequence 0, want 1" {
		t.Fatalf("wrong error message: %v", err)
	}
}

func TestAcceptClientChunkClosedStream(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}
	s := sr.registerWrite(conn)

	sr.remove(s.id)

	err := s.acceptClientChunk(0)
	if err == nil {
		t.Fatal("acceptClientChunk on closed stream should error")
	}
	if err.Error() != "stream is closed" {
		t.Fatalf("wrong error message: %v, want 'stream is closed'", err)
	}
}

func TestAcceptClientChunkZeroSequenceFromStart(t *testing.T) {
	// Verifies chunk at seq 0 works (not just that it's the default)
	sr := newStreamRegistry()
	conn := &wsConnection{}
	s := sr.registerRead(conn)

	if err := s.acceptClientChunk(0); err != nil {
		t.Fatalf("acceptClientChunk(0) on new stream = %v, want nil", err)
	}
}

// --- Multiple connections and streams ---

func TestStreamRegistryMultipleConnections(t *testing.T) {
	sr := newStreamRegistry()
	conn1 := &wsConnection{}
	conn2 := &wsConnection{}

	sr.registerRead(conn1)
	sr.registerWrite(conn1)
	sr.registerProcessOutput(conn2)
	sr.registerWrite(conn2)

	if c := sr.count(); c != 4 {
		t.Fatalf("count = %d, want 4", c)
	}

	// close conn1
	sr.closeConnection(conn1)
	if c := sr.count(); c != 2 {
		t.Fatalf("after closing conn1, count = %d, want 2", c)
	}

	// close conn2
	sr.closeConnection(conn2)
	if c := sr.count(); c != 0 {
		t.Fatalf("after closing conn2, count = %d, want 0", c)
	}
}

// --- StreamRegistry: Concurrent Safety ---

func TestStreamRegistryConcurrentRegisterAndRemove(t *testing.T) {
	sr := newStreamRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn := &wsConnection{}
			s := sr.registerRead(conn)
			sr.lookup(s.id)
			sr.remove(s.id)
		}()
	}
	wg.Wait()
}

func TestStreamRegistryConcurrentReads(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}
	s := sr.registerRead(conn)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sr.lookup(s.id)
			sr.count()
		}()
	}
	wg.Wait()
}

func TestStreamRegistryConcurrentMixedOperations(t *testing.T) {
	sr := newStreamRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn := &wsConnection{}
			sr.registerRead(conn)
			sr.registerWrite(conn)
		}()
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sr.count()
			conn := &wsConnection{}
			sr.registerProcessOutput(conn)
		}()
	}
	wg.Wait()

	// 10 goroutines x 2 registers (read+write) + 10 goroutines x 1 register (process) = 30 total
	if c := sr.count(); c != 30 {
		t.Fatalf("expected 30 streams, got %d", c)
	}
}

// --- writeStreamChunk and writeStreamClose (function-level) ---
// These functions just delegate to writeWSFrame/writeWSMessage.
// They are tested indirectly via the WS integration tests, but we
// verify they accept nil connections without panicking for safety.

func TestWriteStreamHelpersNilConn(t *testing.T) {
	// These should not panic — they return errors via writeWSFrame/writeWSMessage nil checks
	_ = writeStreamChunk(nil, "s1", 0, "stdout", []byte("hello"), false)
	_ = writeStreamClose(nil, nil, "s1", true, nil, "")
}

// --- Edge Cases ---

func TestStreamRegistryLookupAfterCloseConnection(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}
	s := sr.registerRead(conn)

	sr.closeConnection(conn)

	_, ok := sr.lookup(s.id)
	if ok {
		t.Fatal("lookup should not find stream after closeConnection")
	}
}

func TestStreamRegistryRegisterDoesNotReuseIDs(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	s1 := sr.registerRead(conn)
	sr.remove(s1.id)

	s2 := sr.registerRead(conn)
	if s2.id == s1.id {
		t.Fatal("register after remove should produce a new ID")
	}
}

func TestStreamRegistryCloseAllEmpty(t *testing.T) {
	sr := newStreamRegistry()
	ids := sr.closeAll()
	if ids != nil && len(ids) != 0 {
		t.Fatalf("closeAll on empty registry = %v, want nil or empty", ids)
	}
}

func TestStreamRegistryConnectionWithNoStreams(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}

	// closeConnection on a connection with no registered streams
	ids := sr.closeConnection(conn)
	if len(ids) != 0 {
		t.Fatalf("closeConnection on unused conn = %d ids, want 0", len(ids))
	}
}

func TestStreamStateAcceptClientChunkAfterRemove(t *testing.T) {
	// After remove, the stream is marked closed but acceptClientChunk
	// still operates on the (now-closed) streamState pointer
	sr := newStreamRegistry()
	conn := &wsConnection{}
	s := sr.registerRead(conn)

	_ = s.acceptClientChunk(0)
	sr.remove(s.id)

	// accept on closed stream should error with "stream is closed"
	err := s.acceptClientChunk(1)
	if err == nil {
		t.Fatal("expected error on closed stream")
	}
	if err.Error() != "stream is closed" {
		t.Fatalf("error = %q, want 'stream is closed'", err.Error())
	}
}

func TestStreamRegistryRegisterConcurrentSameConnection(t *testing.T) {
	sr := newStreamRegistry()
	conn := &wsConnection{}
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := sr.registerRead(conn)
			if s != nil {
				sr.remove(s.id)
			}
		}()
	}
	wg.Wait()
}
