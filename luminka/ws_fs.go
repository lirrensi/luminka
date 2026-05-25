// FILE: luminka/ws_fs.go
// PURPOSE: Route filesystem websocket events, including byte streams.
// OWNS: Text filesystem requests, byte-stream file opens, and stream chunk handling.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func statToMap(info os.FileInfo) map[string]any {
	return map[string]any{
		"size":       info.Size(),
		"mode":       info.Mode(),
		"mod_time":   info.ModTime().Format(time.RFC3339Nano),
		"is_dir":     info.IsDir(),
		"is_symlink": info.Mode()&os.ModeSymlink != 0,
	}
}

func flagToOpenFlags(flag string) (int, os.FileMode) {
	switch flag {
	case "r":
		return os.O_RDONLY, 0
	case "r+":
		return os.O_RDWR, 0
	case "w":
		return os.O_CREATE | os.O_TRUNC | os.O_WRONLY, 0o644
	case "w+":
		return os.O_CREATE | os.O_TRUNC | os.O_RDWR, 0o644
	case "a":
		return os.O_CREATE | os.O_APPEND | os.O_WRONLY, 0o644
	case "a+":
		return os.O_CREATE | os.O_APPEND | os.O_RDWR, 0o644
	default:
		return os.O_RDONLY, 0
	}
}

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
		if err := rt.FSBridge.WriteBytes(request.Path, []byte(request.dataString())); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		rt.logEvent("fs_write", map[string]any{"path": request.Path})
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
		rt.logEvent("fs_delete", map[string]any{"path": request.Path})
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
	// --- New Node-style operations ---
	case "fs_access":
		err := rt.FSBridge.Access(request.Path, os.FileMode(request.Perm))
		if err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_append_file":
		if err := rt.FSBridge.AppendFile(request.Path, payload); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_chmod":
		if err := rt.FSBridge.Chmod(request.Path, os.FileMode(request.Perm)); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_copy_file":
		if err := rt.FSBridge.CopyFile(request.Src, request.Dest); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		if request.Perm != 0 {
			resolved, err := rt.FSBridge.Realpath(request.Dest)
			if err == nil {
				_ = os.Chmod(resolved, os.FileMode(request.Perm))
			}
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_cp":
		recursive := request.Flag == "recursive"
		if err := rt.FSBridge.Cp(request.Src, request.Dest, recursive); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_link":
		if err := rt.FSBridge.Link(request.Src, request.Dest); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_lstat":
		info, err := rt.FSBridge.Lstat(request.Path)
		if err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeStatResponse(conn, request.ID, true, "", statToMap(info))
	case "fs_mkdir":
		perm := os.FileMode(request.Perm)
		if perm == 0 {
			perm = 0o755
		}
		var err error
		if request.Flag == "recursive" {
			err = rt.FSBridge.MkdirAll(request.Path, perm)
		} else {
			err = rt.FSBridge.Mkdir(request.Path, perm)
		}
		if err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_mkdtemp":
		path, err := rt.FSBridge.Mkdtemp(request.Path)
		if err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeDataResponse(conn, request.ID, true, "", path)
	case "fs_open":
		flags, perm := flagToOpenFlags(request.Flag)
		if request.Perm != 0 {
			perm = os.FileMode(request.Perm)
		}
		file, err := rt.FSBridge.Open(request.Path, flags, perm)
		if err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		if rt.streams == nil {
			file.Close()
			return writeFSResponse(conn, request.ID, false, "stream registry is unavailable", nil, nil, nil)
		}
		stream := rt.streams.registerHandle(conn, file)
		if stream == nil {
			file.Close()
			return writeFSResponse(conn, request.ID, false, "stream registry is unavailable", nil, nil, nil)
		}
		return writeFSStreamResponse(conn, request.ID, true, "", stream.id)
	case "fs_read_file":
		data, err := rt.FSBridge.ReadBytes(request.Path)
		if err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		if request.Flag == "utf8" {
			text := string(data)
			return writeFSResponse(conn, request.ID, true, "", &text, nil, nil)
		}
		// Binary data: send as payload after JSON header
		return writeWSFrame(conn, wsMessage{Event: "fs_response", ID: request.ID, Ok: boolPtr(true)}, data)
	case "fs_readdir":
		entries, err := rt.FSBridge.ReadDir(request.Path)
		if err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		names := make([]string, 0, len(entries))
		types := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
			if entry.IsDir() {
				types = append(types, "directory")
			} else if entry.Type()&os.ModeSymlink != 0 {
				types = append(types, "symlink")
			} else {
				types = append(types, "file")
			}
		}
		return writeFSResponseWithTypes(conn, request.ID, true, "", names, types)
	case "fs_readlink":
		target, err := rt.FSBridge.Readlink(request.Path)
		if err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeDataResponse(conn, request.ID, true, "", target)
	case "fs_realpath":
		resolved, err := rt.FSBridge.Realpath(request.Path)
		if err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeDataResponse(conn, request.ID, true, "", resolved)
	case "fs_rename":
		if err := rt.FSBridge.Rename(request.Path, request.Dest); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_rm":
		if request.Flag == "recursive" {
			if err := rt.FSBridge.RemoveAll(request.Path); err != nil {
				return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
			}
		} else {
			if err := rt.FSBridge.Remove(request.Path); err != nil {
				return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
			}
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_rmdir":
		if err := rt.FSBridge.Rmdir(request.Path); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_stat":
		info, err := rt.FSBridge.Stat(request.Path)
		if err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeStatResponse(conn, request.ID, true, "", statToMap(info))
	case "fs_symlink":
		if err := rt.FSBridge.Symlink(request.Src, request.Path); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_truncate":
		if err := rt.FSBridge.Truncate(request.Path, request.Len); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_unlink":
		if err := rt.FSBridge.Remove(request.Path); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_utimes":
		atime, atimeErr := time.Parse(time.RFC3339Nano, request.Atime)
		mtime, mtimeErr := time.Parse(time.RFC3339Nano, request.Mtime)
		if atimeErr != nil || mtimeErr != nil {
			// Try Unix timestamps
			var atimeSec, mtimeSec int64
			if _, err := fmt.Sscanf(request.Atime, "%d", &atimeSec); err != nil {
				return writeFSResponse(conn, request.ID, false, "invalid atime: "+err.Error(), nil, nil, nil)
			}
			if _, err := fmt.Sscanf(request.Mtime, "%d", &mtimeSec); err != nil {
				return writeFSResponse(conn, request.ID, false, "invalid mtime: "+err.Error(), nil, nil, nil)
			}
			atime = time.Unix(atimeSec, 0)
			mtime = time.Unix(mtimeSec, 0)
		}
		if err := rt.FSBridge.Utimes(request.Path, atime, mtime); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	case "fs_write_file":
		if err := rt.FSBridge.WriteBytes(request.Path, payload); err != nil {
			return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
		}
		rt.logEvent("fs_write", map[string]any{"path": request.Path})
		return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
	// --- Handle operations ---
	case "handle_read":
		return rt.handleHandleRead(conn, request)
	case "handle_write":
		return rt.handleHandleWrite(conn, request, payload)
	case "handle_close":
		return rt.handleHandleClose(conn, request)
	case "handle_stat":
		return rt.handleHandleStat(conn, request)
	case "handle_truncate":
		return rt.handleHandleTruncate(conn, request)
	case "handle_sync":
		return rt.handleHandleSync(conn, request)
	case "handle_datasync":
		return rt.handleHandleDatasync(conn, request)
	case "handle_chmod":
		return rt.handleHandleChmod(conn, request)
	case "handle_utimes":
		return rt.handleHandleUtimes(conn, request)
	default:
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("unknown event %q", request.Event))
	}
}

// handleFSOpenRead is the legacy blocking single-file read path.
// Prefer the streaming API (fs_open with stream_chunk) for large files or
// when cancellation is needed. This handler blocks the WebSocket message
// loop for the entire duration of the read.
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
			if err := writeStreamChunk(conn, stream.id, seq, "", buf[:n], eof, nil); err != nil {
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
	if stream.conn != conn {
		return writeErrorResponse(conn, request.ID, "stream not owned by this connection")
	}
	if stream.kind != streamKindWrite {
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("stream %q is not writable", request.StreamID))
	}
	// Check payload before advancing sequence — empty payload is a no-op.
	if len(payload) == 0 {
		return nil
	}
	if err := stream.acceptClientChunk(request.Seq); err != nil {
		return writeErrorResponse(conn, request.ID, err.Error())
	}
	// Lock per-stream mutex to protect file access.
	stream.mu.Lock()
	if stream.file == nil {
		stream.mu.Unlock()
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("stream %q is not writable", request.StreamID))
	}
	if _, err := stream.file.Write(payload); err != nil {
		stream.mu.Unlock()
		rt.streams.remove(stream.id)
		return writeStreamClose(conn, nil, stream.id, false, nil, err.Error())
	}
	stream.mu.Unlock()
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
	if stream.conn != conn {
		return writeStreamClose(conn, request.ID, stream.id, false, nil, "stream not owned by this connection")
	}
	if stream.kind != streamKindWrite {
		rt.streams.remove(stream.id)
		return writeStreamClose(conn, request.ID, stream.id, false, nil, fmt.Sprintf("stream %q is not writable", request.StreamID))
	}
	rt.streams.remove(stream.id)
	return writeStreamClose(conn, request.ID, stream.id, true, nil, "")
}

// --- Handle operation handlers ---

func (rt *Runtime) handleHandleRead(conn *wsConnection, request wsMessage) error {
	if rt.streams == nil {
		return writeErrorResponse(conn, request.ID, "stream registry is unavailable")
	}
	stream, ok := rt.streams.lookup(request.StreamID)
	if !ok || stream == nil || stream.file == nil {
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("handle %q is not open", request.StreamID))
	}
	if stream.conn != conn {
		return writeErrorResponse(conn, request.ID, "stream not owned by this connection")
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()

	var data []byte
	var readErr error

	// Backward compat: if Offset/Length (new pointer fields) are nil, fall back to
	// Len (offset) and Perm (length) which the old SDK sent.
	offset := request.Offset
	length := request.Length
	if offset == nil && request.Len != 0 {
		o := request.Len
		offset = &o
	}
	if length == nil && request.Perm != 0 {
		l := int64(request.Perm)
		length = &l
	}

	hasOffset := offset != nil
	hasLength := length != nil

	switch {
	case hasOffset && hasLength:
		// ReadAt with explicit offset and length
		buf := make([]byte, *length)
		var n int
		n, readErr = stream.file.ReadAt(buf, *offset)
		data = buf[:n]
	case hasOffset && !hasLength:
		// Seek to offset, then ReadAll
		if _, err := stream.file.Seek(*offset, 0); err != nil {
			return writeFSResponse(conn, request.ID, false, fmt.Sprintf("seek: %v", err), nil, nil, nil)
		}
		data, readErr = io.ReadAll(stream.file)
	case !hasOffset && hasLength:
		// Read N bytes from current position
		buf := make([]byte, *length)
		var n int
		n, readErr = stream.file.Read(buf)
		data = buf[:n]
	default:
		// ReadAll from position 0 (legacy/stream behavior — matches Node.js filehandle.readFile)
		if _, err := stream.file.Seek(0, 0); err != nil {
			return writeFSResponse(conn, request.ID, false, fmt.Sprintf("seek: %v", err), nil, nil, nil)
		}
		data, readErr = io.ReadAll(stream.file)
	}

	if readErr != nil && readErr != io.EOF {
		return writeFSResponse(conn, request.ID, false, fmt.Sprintf("read: %v", readErr), nil, nil, nil)
	}

	if len(data) > 0 {
		return writeWSFrame(conn, wsMessage{Event: "fs_response", ID: request.ID, Ok: boolPtr(true), StreamID: stream.id}, data)
	}
	return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
}

func (rt *Runtime) handleHandleChmod(conn *wsConnection, request wsMessage) error {
	if rt.streams == nil {
		return writeErrorResponse(conn, request.ID, "stream registry is unavailable")
	}
	stream, ok := rt.streams.lookup(request.StreamID)
	if !ok || stream == nil || stream.file == nil {
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("handle %q is not open", request.StreamID))
	}
	if stream.conn != conn {
		return writeErrorResponse(conn, request.ID, "stream not owned by this connection")
	}
	stream.mu.Lock()
	path := stream.file.Name()
	stream.mu.Unlock()
	if path == "" {
		return writeFSResponse(conn, request.ID, false, "cannot determine file path", nil, nil, nil)
	}
	// Convert absolute path to relative for FSBridge
	rel, err := filepath.Rel(rt.Root, path)
	if err != nil {
		return writeFSResponse(conn, request.ID, false, fmt.Sprintf("path outside root: %v", err), nil, nil, nil)
	}
	if err := rt.FSBridge.Chmod(rel, os.FileMode(request.Perm)); err != nil {
		return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
	}
	return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
}

func (rt *Runtime) handleHandleUtimes(conn *wsConnection, request wsMessage) error {
	if rt.streams == nil {
		return writeErrorResponse(conn, request.ID, "stream registry is unavailable")
	}
	stream, ok := rt.streams.lookup(request.StreamID)
	if !ok || stream == nil || stream.file == nil {
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("handle %q is not open", request.StreamID))
	}
	if stream.conn != conn {
		return writeErrorResponse(conn, request.ID, "stream not owned by this connection")
	}
	atime, atimeErr := time.Parse(time.RFC3339Nano, request.Atime)
	mtime, mtimeErr := time.Parse(time.RFC3339Nano, request.Mtime)
	if atimeErr != nil || mtimeErr != nil {
		var atimeSec, mtimeSec int64
		if _, err := fmt.Sscanf(request.Atime, "%d", &atimeSec); err != nil {
			return writeFSResponse(conn, request.ID, false, "invalid atime: "+err.Error(), nil, nil, nil)
		}
		if _, err := fmt.Sscanf(request.Mtime, "%d", &mtimeSec); err != nil {
			return writeFSResponse(conn, request.ID, false, "invalid mtime: "+err.Error(), nil, nil, nil)
		}
		atime = time.Unix(atimeSec, 0)
		mtime = time.Unix(mtimeSec, 0)
	}
	stream.mu.Lock()
	path := stream.file.Name()
	stream.mu.Unlock()
	if path == "" {
		return writeFSResponse(conn, request.ID, false, "cannot determine file path", nil, nil, nil)
	}
	// Convert absolute path to relative for FSBridge
	rel, err := filepath.Rel(rt.Root, path)
	if err != nil {
		return writeFSResponse(conn, request.ID, false, fmt.Sprintf("path outside root: %v", err), nil, nil, nil)
	}
	if err := rt.FSBridge.Utimes(rel, atime, mtime); err != nil {
		return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
	}
	return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
}

func (rt *Runtime) handleHandleWrite(conn *wsConnection, request wsMessage, payload []byte) error {
	if rt.streams == nil {
		return writeErrorResponse(conn, request.ID, "stream registry is unavailable")
	}
	stream, ok := rt.streams.lookup(request.StreamID)
	if !ok || stream == nil || stream.file == nil {
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("handle %q is not open", request.StreamID))
	}
	if stream.conn != conn {
		return writeErrorResponse(conn, request.ID, "stream not owned by this connection")
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()

	// Backward compat: if Offset (new pointer field) is nil, fall back to Len.
	offset := request.Offset
	if offset == nil && request.Len != 0 {
		o := request.Len
		offset = &o
	}

	if offset != nil {
		// Explicit positional write (WriteAt)
		if _, err := stream.file.WriteAt(payload, *offset); err != nil {
			return writeFSResponse(conn, request.ID, false, fmt.Sprintf("write: %v", err), nil, nil, nil)
		}
	} else {
		// Append write (Write)
		if _, err := stream.file.Write(payload); err != nil {
			return writeFSResponse(conn, request.ID, false, fmt.Sprintf("write: %v", err), nil, nil, nil)
		}
	}
	return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
}

func (rt *Runtime) handleHandleClose(conn *wsConnection, request wsMessage) error {
	if rt.streams == nil {
		return writeErrorResponse(conn, request.ID, "stream registry is unavailable")
	}
	stream, ok := rt.streams.lookup(request.StreamID)
	if !ok || stream == nil {
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("handle %q is not open", request.StreamID))
	}
	if stream.conn != conn {
		return writeErrorResponse(conn, request.ID, "stream not owned by this connection")
	}
	// Lock to safely get id before removeLocked takes the per-stream lock.
	// remove() will acquire registry + per-stream locks internally.
	rt.streams.remove(stream.id)
	return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
}

func (rt *Runtime) handleHandleStat(conn *wsConnection, request wsMessage) error {
	if rt.streams == nil {
		return writeErrorResponse(conn, request.ID, "stream registry is unavailable")
	}
	stream, ok := rt.streams.lookup(request.StreamID)
	if !ok || stream == nil || stream.file == nil {
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("handle %q is not open", request.StreamID))
	}
	if stream.conn != conn {
		return writeErrorResponse(conn, request.ID, "stream not owned by this connection")
	}
	stream.mu.Lock()
	info, err := stream.file.Stat()
	stream.mu.Unlock()
	if err != nil {
		return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
	}
	return writeStatResponse(conn, request.ID, true, "", statToMap(info))
}

func (rt *Runtime) handleHandleTruncate(conn *wsConnection, request wsMessage) error {
	if rt.streams == nil {
		return writeErrorResponse(conn, request.ID, "stream registry is unavailable")
	}
	stream, ok := rt.streams.lookup(request.StreamID)
	if !ok || stream == nil || stream.file == nil {
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("handle %q is not open", request.StreamID))
	}
	if stream.conn != conn {
		return writeErrorResponse(conn, request.ID, "stream not owned by this connection")
	}
	truncLen := request.Len
	if request.Length != nil {
		truncLen = *request.Length
	}
	stream.mu.Lock()
	err := stream.file.Truncate(truncLen)
	stream.mu.Unlock()
	if err != nil {
		return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
	}
	return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
}

func (rt *Runtime) handleHandleSync(conn *wsConnection, request wsMessage) error {
	if rt.streams == nil {
		return writeErrorResponse(conn, request.ID, "stream registry is unavailable")
	}
	stream, ok := rt.streams.lookup(request.StreamID)
	if !ok || stream == nil || stream.file == nil {
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("handle %q is not open", request.StreamID))
	}
	if stream.conn != conn {
		return writeErrorResponse(conn, request.ID, "stream not owned by this connection")
	}
	stream.mu.Lock()
	err := stream.file.Sync()
	stream.mu.Unlock()
	if err != nil {
		return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
	}
	return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
}

func (rt *Runtime) handleHandleDatasync(conn *wsConnection, request wsMessage) error {
	if rt.streams == nil {
		return writeErrorResponse(conn, request.ID, "stream registry is unavailable")
	}
	stream, ok := rt.streams.lookup(request.StreamID)
	if !ok || stream == nil || stream.file == nil {
		return writeErrorResponse(conn, request.ID, fmt.Sprintf("handle %q is not open", request.StreamID))
	}
	if stream.conn != conn {
		return writeErrorResponse(conn, request.ID, "stream not owned by this connection")
	}
	stream.mu.Lock()
	err := fdatasync(stream.file)
	stream.mu.Unlock()
	if err != nil {
		return writeFSResponse(conn, request.ID, false, err.Error(), nil, nil, nil)
	}
	return writeFSResponse(conn, request.ID, true, "", nil, nil, nil)
}
