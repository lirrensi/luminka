// FILE: luminka/stream_exec.go
// PURPOSE: Stream subprocess stdout and stderr into binary websocket chunks.
// OWNS: Shared chunk sequencing and reader-to-stream pumping helpers.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"encoding/json"
	"io"
	"os/exec"
	"sync"
)

type execStreamWriter struct {
	conn     *wsConnection
	streamID string
	id       json.RawMessage // request_id to include in each chunk
	mu       sync.Mutex
	seq      uint64
}

func newExecStreamWriter(conn *wsConnection, streamID string, id json.RawMessage) *execStreamWriter {
	return &execStreamWriter{conn: conn, streamID: streamID, id: id}
}

func (w *execStreamWriter) writeChunk(lane string, payload []byte, eof bool) error {
	if w == nil {
		return io.ErrClosedPipe
	}
	w.mu.Lock()
	seq := w.seq
	w.seq++
	w.mu.Unlock()
	return writeStreamChunk(w.conn, w.streamID, seq, lane, payload, eof, w.id)
}

func pumpReaderToStream(r io.Reader, lane string, writer *execStreamWriter, errCh chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()
	if r == nil {
		return
	}
	buf := make([]byte, fsStreamChunkSize)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			eof := err == io.EOF
			if werr := writer.writeChunk(lane, buf[:n], eof); werr != nil {
				errCh <- werr
				return
			}
		}
		if err == io.EOF {
			return
		}
		if err != nil {
			errCh <- err
			return
		}
	}
}

func firstStreamError(errCh <-chan error) error {
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func commandExitCode(err error) int {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}
