// FILE: luminka/stream.go
// PURPOSE: Track websocket stream lifecycle and binary stream frame helpers.
// OWNS: Stream registration, ownership cleanup, sequence tracking, and stream frame writers.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

type streamKind string

const (
	streamKindRead          streamKind = "read"
	streamKindWrite         streamKind = "write"
	streamKindProcessOutput streamKind = "process_output"
)

type streamRegistry struct {
	mu           sync.Mutex
	nextID       uint64
	streams      map[string]*streamState
	byConnection map[*wsConnection]map[string]struct{}
}

type streamState struct {
	id      string
	kind    streamKind
	conn    *wsConnection
	file    *os.File
	nextSeq uint64
	closed  bool
}

func newStreamRegistry() *streamRegistry {
	return &streamRegistry{
		streams:      make(map[string]*streamState),
		byConnection: make(map[*wsConnection]map[string]struct{}),
	}
}

func (sr *streamRegistry) registerRead(conn *wsConnection) *streamState {
	return sr.register(conn, streamKindRead)
}

func (sr *streamRegistry) registerWrite(conn *wsConnection) *streamState {
	return sr.register(conn, streamKindWrite)
}

func (sr *streamRegistry) registerProcessOutput(conn *wsConnection) *streamState {
	return sr.register(conn, streamKindProcessOutput)
}

func (sr *streamRegistry) register(conn *wsConnection, kind streamKind) *streamState {
	if sr == nil || conn == nil {
		return nil
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.nextID++
	state := &streamState{id: fmt.Sprintf("stream-%d", sr.nextID), kind: kind, conn: conn}
	sr.streams[state.id] = state
	if sr.byConnection[conn] == nil {
		sr.byConnection[conn] = make(map[string]struct{})
	}
	sr.byConnection[conn][state.id] = struct{}{}
	return state
}

func (sr *streamRegistry) lookup(id string) (*streamState, bool) {
	if sr == nil {
		return nil, false
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	state, ok := sr.streams[id]
	return state, ok
}

func (sr *streamRegistry) count() int {
	if sr == nil {
		return 0
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return len(sr.streams)
}

func (s *streamState) acceptClientChunk(seq uint64) error {
	if s == nil {
		return errors.New("stream is required")
	}
	if s.closed {
		return errors.New("stream is closed")
	}
	if seq != s.nextSeq {
		return fmt.Errorf("unexpected stream sequence %d, want %d", seq, s.nextSeq)
	}
	s.nextSeq++
	return nil
}

func (s *streamState) attachFile(file *os.File) {
	if s == nil {
		return
	}
	s.file = file
}

func (s *streamState) closeResource() {
	if s == nil || s.file == nil {
		return
	}
	_ = s.file.Close()
	s.file = nil
}

func (sr *streamRegistry) remove(id string) {
	if sr == nil || id == "" {
		return
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.removeLocked(id)
}

func (sr *streamRegistry) closeConnection(conn *wsConnection) []string {
	if sr == nil || conn == nil {
		return nil
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	ids := sr.connectionIDsLocked(conn)
	for _, id := range ids {
		sr.removeLocked(id)
	}
	return ids
}

func (sr *streamRegistry) closeAll() []string {
	if sr == nil {
		return nil
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	ids := make([]string, 0, len(sr.streams))
	for id := range sr.streams {
		ids = append(ids, id)
	}
	for _, id := range ids {
		sr.removeLocked(id)
	}
	return ids
}

func (sr *streamRegistry) connectionIDsLocked(conn *wsConnection) []string {
	ids := sr.byConnection[conn]
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	return out
}

func (sr *streamRegistry) removeLocked(id string) {
	state, ok := sr.streams[id]
	if !ok {
		return
	}
	state.closed = true
	state.closeResource()
	delete(sr.streams, id)
	if ids := sr.byConnection[state.conn]; ids != nil {
		delete(ids, id)
		if len(ids) == 0 {
			delete(sr.byConnection, state.conn)
		}
	}
}

func writeStreamChunk(conn *wsConnection, streamID string, seq uint64, lane string, payload []byte, eof bool) error {
	return writeWSFrame(conn, wsMessage{Event: "stream_chunk", StreamID: streamID, Seq: seq, Lane: lane, EOF: eof}, payload)
}

func writeStreamClose(conn *wsConnection, id json.RawMessage, streamID string, ok bool, code *int, errMsg string) error {
	return writeWSMessage(conn, wsMessage{Event: "stream_close", ID: id, StreamID: streamID, Ok: boolPtr(ok), Code: code, Error: errMsg})
}
