// FILE: luminka/ws_fs.go
// PURPOSE: Route filesystem websocket events, including byte streams.
// OWNS: Text filesystem requests, byte-stream file opens, and stream chunk handling.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"fmt"
	"io"
)

const fsStreamChunkSize = 32 * 1024

func (rt *Runtime) handleFilesystemRequest(conn *wsConnection, request wsMessage, payload []byte) error {
	if rt == nil {
		return writeErrorResponse(conn, request.ID, "runtime is required")
	}
	if !rt.Capabilities.FS {
		return writeFSResponse(conn, request.ID, false, "filesystem capability is disabled", nil, nil, nil)
	}
	if rt.FSBridge == nil {
		return writeFSResponse(conn, request.ID, false, "filesystem bridge is unavailable", nil, nil, nil)
	}

	switch request.Event {
	case "fs_read_text":
		data, err := rt.FSBridge.ReadBytes(request.Path)
		if err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		text := string(data)
		return writeFSResponse(conn, request.ID, true, "", &text, nil, nil)
	case "fs_write_text":
		if err := rt.FSBridge.WriteBytes(request.Path, []byte(request.Data)); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_list":
		files, err := rt.FSBridge.List(request.Path)
		if err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, files, nil)
	case "fs_delete":
		if err := rt.FSBridge.Delete(request.Path); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_exists":
		exists, err := rt.FSBridge.Exists(request.Path)
		if err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, boolPtr(exists))
	case "fs_watch":
		if rt.Watcher == nil {
			return writeFSResponse(conn, request.ID, false, "watcher is unavailable", nil, nil, nil)
		}
		if err := rt.Watcher.Add(request.Path); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		rt.Watcher.Start()
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_unwatch":
		if rt.Watcher == nil {
			return writeFSResponse(conn, request.ID, false, "watcher is unavailable", nil, nil, nil)
		}
		if err := rt.Watcher.Remove(request.Path); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_open_read":
		return rt.handleFSOpenRead(conn, request)
	case "fs_open_write":
		return rt.handleFSOpenWrite(conn, request)
	case "stream_chunk":
		return rt.handleStreamChunk(conn, request, payload)
	case "stream_close":
		return rt.handleStreamClose(conn, request)
	default:
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("unknown event %q", request.Event))
	}
}

func (rt *Runtime) handleFSOpenRead(conn *wsConnection, request wsMessage) error {
	if rt.streams == nil {
		return writeFSResponse(conn, request.ID, false, "stream registry is unavailable", nil, nil, nil)
	}
	stream := rt.streams.registerRead(conn)
	if stream == nil {
		return writeFSResponse(conn, request.ID, false, "stream registry is unavailable", nil, nil, nil)
	}
	file, _, err := rt.FSBridge.OpenRead(request.Path)
	if err != nil {
		rt.streams.remove(stream.id)
		return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
	}
	stream.attachFile(file)
	if err := writeFSStreamResponse(conn, request.ID, true, "", stream.id); err != nil {
		rt.streams.remove(stream.id)
		return err
	}
	defer rt.streams.remove(stream.id)

	buf := make([]byte, fsStreamChunkSize)
	var seq uint64
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			eof := readErr == io.EOF
			if err := writeStreamChunk(conn, stream.id, seq, "", buf[:n], eof); err != nil {
				return err
			}
			seq++
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return writeStreamClose(conn, nil, stream.id, false, nil, readErr.Error())
		}
	}
	return writeStreamClose(conn, nil, stream.id, true, nil, "")
}

func (rt *Runtime) handleFSOpenWrite(conn *wsConnection, request wsMessage) error {
	if rt.streams == nil {
		return writeFSResponse(conn, request.ID, false, "stream registry is unavailable", nil, nil, nil)
	}
	stream := rt.streams.registerWrite(conn)
	if stream == nil {
		return writeFSResponse(conn, request.ID, false, "stream registry is unavailable", nil, nil, nil)
	}
	file, err := rt.FSBridge.OpenWrite(request.Path)
	if err != nil {
		rt.streams.remove(stream.id)
		return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
	}
	stream.attachFile(file)
	if err := writeFSStreamResponse(conn, request.ID, true, "", stream.id); err != nil {
		rt.streams.remove(stream.id)
		return err
	}
	return nil
}

func (rt *Runtime) handleStreamChunk(conn *wsConnection, request wsMessage, payload []byte) error {
	if rt.streams == nil {
		return writeErrorResponse(conn, request.ID, "stream registry is unavailable")
	}
	stream, ok := rt.streams.lookup(request.StreamID)
	if !ok || stream == nil {
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("stream %q is not open", request.StreamID))
	}
	if stream.kind != streamKindWrite {
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("stream %q is not writable", request.StreamID))
	}
	if err := stream.acceptClientChunk(request.Seq); err != nil {
		return writeErrorResponse(conn, request.ID, err.Error())
	}
	if stream.file == nil {
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("stream %q is not writable", request.StreamID))
	}
	if len(payload) == 0 {
		return nil
	}
	if _, err := stream.file.Write(payload); err != nil {
		rt.streams.remove(stream.id)
		return writeStreamClose(conn, nil, stream.id, false, nil, err.Error())
	}
	return nil
}

func (rt *Runtime) handleStreamClose(conn *wsConnection, request wsMessage) error {
	if rt.streams == nil {
		return writeStreamClose(conn, request.ID, request.StreamID, false, nil, "stream registry is unavailable")
	}
	stream, ok := rt.streams.lookup(request.StreamID)
	if !ok || stream == nil {
		return writeStreamClose(conn, request.ID, request.StreamID, false, nil, fmt.Sprintf("stream %q is not open", request.StreamID))
	}
	if stream.kind != streamKindWrite {
		rt.streams.remove(stream.id)
		return writeStreamClose(conn, request.ID, stream.id, false, nil, fmt.Sprintf("stream %q is not writable", request.StreamID))
	}
	rt.streams.remove(stream.id)
	return writeStreamClose(conn, request.ID, stream.id, true, nil, "")
}
