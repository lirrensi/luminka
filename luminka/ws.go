// FILE: luminka/ws.go
// PURPOSE: Serve the canonical Phase 2 websocket protocol and capability dispatch.
// OWNS: Connection tracking, request routing, capability gating, and push notifications.
// EXPORTS: WSMessage, WSConnection
// DOCS: docs/spec.md, docs/arch.md

package luminka

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WSMessage struct {
	Event           string          `json:"event"`
	ID              json.RawMessage `json:"id,omitempty"`
	Ok              *bool           `json:"ok,omitempty"`
	Error           string          `json:"error,omitempty"`
	Path            string          `json:"path,omitempty"`
	Src             string          `json:"src,omitempty"`
	Dest            string          `json:"dest,omitempty"`
	Data            json.RawMessage `json:"data,omitempty"`
	Channel         string          `json:"channel,omitempty"`
	ContentType     string          `json:"content_type,omitempty"`
	Echo            bool            `json:"echo,omitempty"`
	Files           []string        `json:"files,omitempty"`
	FileTypes       []string        `json:"file_types,omitempty"`
	Exists          *bool           `json:"exists,omitempty"`
	Runner          string          `json:"runner,omitempty"`
	File            string          `json:"file,omitempty"`
	Cmd             string          `json:"cmd,omitempty"`
	Args            []string        `json:"args,omitempty"`
	Timeout         int             `json:"timeout,omitempty"`
	Stdout          string          `json:"stdout,omitempty"`
	Stderr          string          `json:"stderr,omitempty"`
	Code            *int            `json:"code,omitempty"`
	Name            string          `json:"name,omitempty"`
	AppVersion      string          `json:"app_version,omitempty"`
	RuntimeVersion  string          `json:"runtime_version,omitempty"`
	ProtocolVersion string          `json:"protocol_version,omitempty"`
	Mode            Mode            `json:"mode,omitempty"`
	Root            string          `json:"root,omitempty"`
	StreamID        string          `json:"stream_id,omitempty"`
	Seq             uint64          `json:"seq,omitempty"`
	Lane            string          `json:"lane,omitempty"`
	EOF             bool            `json:"eof,omitempty"`
	Capabilities    capabilityState `json:"capabilities,omitempty"`
	Flag            string          `json:"flag,omitempty"`
	Perm            uint32          `json:"perm,omitempty"`
	Atime           string          `json:"atime,omitempty"`
	Mtime           string          `json:"mtime,omitempty"`
	Len             int64           `json:"len,omitempty"`
	Offset          *int64          `json:"offset,omitempty"`
	Length          *int64          `json:"length,omitempty"`
	Stat            json.RawMessage `json:"stat,omitempty"`
	HandleID        string          `json:"handle_id,omitempty"`
	Payload         []byte          `json:"-"`
}

type websocketConn interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(int, []byte) error
	Close() error
}

type WSConnection struct {
	conn          websocketConn
	writeMu       sync.Mutex
	authenticated bool
}

func (m WSMessage) MarshalJSON() ([]byte, error) {
	type wire struct {
		Event           string           `json:"event"`
		ID              json.RawMessage  `json:"id,omitempty"`
		Ok              *bool            `json:"ok,omitempty"`
		Error           string           `json:"error,omitempty"`
		Path            string           `json:"path,omitempty"`
		Src             string           `json:"src,omitempty"`
		Dest            string           `json:"dest,omitempty"`
		Data            json.RawMessage  `json:"data,omitempty"`
		Channel         string           `json:"channel,omitempty"`
		ContentType     string           `json:"content_type,omitempty"`
		Echo            bool             `json:"echo,omitempty"`
		Files           []string         `json:"files,omitempty"`
		FileTypes       []string         `json:"file_types,omitempty"`
		Exists          *bool            `json:"exists,omitempty"`
		Runner          string           `json:"runner,omitempty"`
		File            string           `json:"file,omitempty"`
		Cmd             string           `json:"cmd,omitempty"`
		Args            []string         `json:"args,omitempty"`
		Timeout         int              `json:"timeout,omitempty"`
		Stdout          string           `json:"stdout,omitempty"`
		Stderr          string           `json:"stderr,omitempty"`
		Code            *int             `json:"code,omitempty"`
		Name            string           `json:"name,omitempty"`
		AppVersion      string           `json:"app_version,omitempty"`
		RuntimeVersion  string           `json:"runtime_version,omitempty"`
		ProtocolVersion string           `json:"protocol_version,omitempty"`
		Mode            Mode             `json:"mode,omitempty"`
		Root            string           `json:"root,omitempty"`
		StreamID        string           `json:"stream_id,omitempty"`
		Seq             uint64           `json:"seq,omitempty"`
		Lane            string           `json:"lane,omitempty"`
		EOF             bool             `json:"eof,omitempty"`
		Capabilities    *capabilityState `json:"capabilities,omitempty"`
		Flag            string           `json:"flag,omitempty"`
		Perm            uint32           `json:"perm,omitempty"`
		Atime           string           `json:"atime,omitempty"`
		Mtime           string           `json:"mtime,omitempty"`
		Len             int64            `json:"len,omitempty"`
		Offset          *int64           `json:"offset,omitempty"`
		Length          *int64           `json:"length,omitempty"`
		Stat            json.RawMessage  `json:"stat,omitempty"`
		HandleID        string           `json:"handle_id,omitempty"`
	}
	out := wire{
		Event:           m.Event,
		ID:              m.ID,
		Ok:              m.Ok,
		Error:           m.Error,
		Path:            m.Path,
		Src:             m.Src,
		Dest:            m.Dest,
		Data:            m.Data,
		Channel:         m.Channel,
		ContentType:     m.ContentType,
		Echo:            m.Echo,
		Files:           m.Files,
		FileTypes:       m.FileTypes,
		Exists:          m.Exists,
		Runner:          m.Runner,
		File:            m.File,
		Cmd:             m.Cmd,
		Args:            m.Args,
		Timeout:         m.Timeout,
		Stdout:          m.Stdout,
		Stderr:          m.Stderr,
		Code:            m.Code,
		Name:            m.Name,
		AppVersion:      m.AppVersion,
		RuntimeVersion:  m.RuntimeVersion,
		ProtocolVersion: m.ProtocolVersion,
		Mode:            m.Mode,
		Root:            m.Root,
		StreamID:        m.StreamID,
		Seq:             m.Seq,
		Lane:            m.Lane,
		EOF:             m.EOF,
		Flag:            m.Flag,
		Perm:            m.Perm,
		Atime:           m.Atime,
		Mtime:           m.Mtime,
		Len:             m.Len,
		Offset:          m.Offset,
		Length:          m.Length,
		Stat:            m.Stat,
		HandleID:        m.HandleID,
	}
	if m.Event == "app_info" || m.Event == "response:app:info" || m.Capabilities != (capabilityState{}) {
		caps := m.Capabilities
		out.Capabilities = &caps
	}
	return json.Marshal(out)
}

func (m WSMessage) dataString() string {
	if len(m.Data) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(m.Data, &value); err == nil {
		return value
	}
	return string(m.Data)
}

func rawStringData(value string) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}

func (rt *Runtime) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(req *http.Request) bool {
			return websocketOriginAllowed(req)
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(128 * 1024 * 1024) // 128 MB

	wsConn := rt.registerConnection(conn)
	defer func() {
		rt.unregisterConnection(wsConn)
		_ = conn.Close()
	}()
	rt.handleWebSocketSession(wsConn)
}

func (rt *Runtime) handleWebSocketSession(wsConn *WSConnection) {
	if rt == nil || wsConn == nil {
		return
	}

	for {
		msgType, request, payload, err := readWSFrame(wsConn)
		if err != nil {
			if msgType == websocket.BinaryMessage {
				_ = WriteErrorResponse(wsConn, nil, err.Error())
				continue
			}
			return
		}
		if msgType == websocket.TextMessage {
			_ = WriteErrorResponse(wsConn, nil, "websocket text frames are not supported")
			continue
		}
		if request.Event == "" {
			_ = WriteErrorResponse(wsConn, request.ID, "message event is required")
			continue
		}

		if rt.Mode == ModeWebview && !wsConn.authenticated && request.Event != "ws:auth" {
			_ = WriteErrorResponse(wsConn, request.ID, "authentication required")
			rt.logEvent("error", map[string]any{"message": "unauthenticated message rejected"})
			return
		}

		// Check custom handlers before built-in dispatch
		rt.handlerMu.RLock()
		entry, hasCustom := rt.customHandlers[request.Event]
		rt.handlerMu.RUnlock()

		if hasCustom {
			reqCopy := request // copy so handler can't mutate the loop variable
			reqCopy.Payload = payload
			err := entry.handler(wsConn, &reqCopy)
			if err == nil {
				continue // handler took responsibility
			}
			if !errors.Is(err, ErrUnhandled) {
				// Handler returned an unexpected error
				_ = WriteErrorResponse(wsConn, request.ID, err.Error())
				continue
			}
			// ErrUnhandled → fall through to built-in switch
		}

		switch request.Event {
		case "ws:auth":
			_ = rt.handleWSAuth(wsConn, request)
			continue
		case "app:info":
			_ = WriteWSMessage(wsConn, WSMessage{
				Event:           "response:app:info",
				ID:              request.ID,
				Ok:              boolPtr(true),
				Name:            rt.Name,
				AppVersion:      rt.AppVersion,
				RuntimeVersion:  RuntimeVersion,
				ProtocolVersion: ProtocolVersion,
				Mode:            rt.Mode,
				Root:            rt.Root,
				Capabilities:    rt.Capabilities,
			})
		case "file:read_text", "file:write_text", "file:list", "file:delete", "file:exists",
			"file:watch", "file:unwatch", "file:open_read", "file:open_write",
			"stream:chunk", "stream:close",
			"file:access", "file:append_file", "file:chmod", "file:copy_file", "file:cp",
			"file:link", "file:lstat", "file:mkdir", "file:mkdtemp", "file:open",
			"file:read_file", "file:readdir", "file:readlink", "file:realpath",
			"file:rename", "file:rm", "file:rmdir", "file:stat", "file:symlink",
			"file:truncate", "file:unlink", "file:utimes", "file:write_file",
			"handle:read", "handle:write", "handle:close", "handle:stat",
			"handle:truncate", "handle:sync", "handle:datasync",
			"handle:chmod", "handle:utimes":
			_ = rt.handleFilesystemRequest(wsConn, request, payload)
		case "ws:broadcast":
			_ = rt.handleBroadcastRequest(wsConn, request, payload)
		case "script:exec":
			_ = rt.handleScriptRequest(wsConn, request)
		case "script:stream":
			_ = rt.handleScriptStreamRequest(wsConn, request)
		case "shell:exec":
			_ = rt.handleShellRequest(wsConn, request)
		case "shell:stream":
			_ = rt.handleShellStreamRequest(wsConn, request)
		default:
			rt.logEvent("error", map[string]any{"message": fmt.Sprintf("unknown event %q", request.Event)})
			_ = WriteErrorResponse(wsConn, request.ID, fmt.Sprintf("unknown event %q", request.Event))
		}
	}
}

func websocketOriginAllowed(r *http.Request) bool {
	if r == nil {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if originURL.Scheme != "http" && originURL.Scheme != "https" {
		return false
	}
	originHost, originPort, err := net.SplitHostPort(originURL.Host)
	if err != nil {
		return false
	}
	if !isLoopbackOriginHost(originHost) {
		return false
	}
	requestHost, requestPort, err := net.SplitHostPort(r.Host)
	if err != nil {
		return false
	}
	return originHost == requestHost && originPort == requestPort
}

func isLoopbackOriginHost(host string) bool {
	if host == "localhost" || host == "127.0.0.1" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (rt *Runtime) registerConnection(conn websocketConn) *WSConnection {
	if rt == nil || conn == nil {
		return nil
	}
	wsConn := &WSConnection{conn: conn}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.connections == nil {
		rt.connections = make(map[*WSConnection]struct{})
	}
	rt.connections[wsConn] = struct{}{}
	if rt.idleTimer != nil {
		rt.idleTimer.Stop()
		rt.idleTimer = nil
	}
	rt.logEvent("ws_connect", map[string]any{
		"connections": len(rt.connections),
	})
	return wsConn
}

func (rt *Runtime) unregisterConnection(conn *WSConnection) {
	if rt == nil || conn == nil {
		return
	}
	if rt.streams != nil {
		rt.streams.closeConnection(conn)
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.connections, conn)
	rt.logEvent("ws_disconnect", map[string]any{
		"connections": len(rt.connections),
	})
	if len(rt.connections) == 0 {
		if rt.idleTimer != nil {
			rt.idleTimer.Stop()
			rt.idleTimer = nil
		}
		rt.startIdleTimerLocked()
	}
}

func (rt *Runtime) startIdleTimerLocked() {
	if rt == nil {
		return
	}
	idle := rt.Idle
	if idle == 0 {
		idle = defaultIdleTimeout
	}
	if len(rt.connections) != 0 {
		return
	}
	rt.idleTimer = time.AfterFunc(idle, func() {
		if rt.connectionCount() == 0 {
			rt.requestShutdown()
		}
	})
}

func (rt *Runtime) connectionSnapshot() []*WSConnection {
	if rt == nil {
		return nil
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.connections) == 0 {
		return nil
	}
	out := make([]*WSConnection, 0, len(rt.connections))
	for conn := range rt.connections {
		out = append(out, conn)
	}
	return out
}

func (rt *Runtime) connectionSnapshotExcluding(excluded *WSConnection) []*WSConnection {
	if rt == nil {
		return nil
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	out := make([]*WSConnection, 0, len(rt.connections))
	for conn := range rt.connections {
		if conn != excluded {
			out = append(out, conn)
		}
	}
	return out
}

func (rt *Runtime) handleScriptRequest(conn *WSConnection, request WSMessage) error {
	if rt == nil {
		return WriteErrorResponse(conn, request.ID, "runtime is required")
	}
	if !rt.Capabilities.Scripts {
		return WriteExecResponse(conn, "response:script:exec", request.ID, false, "script capability is disabled", "", "", nil)
	}
	if rt.ScriptBridge == nil {
		return WriteExecResponse(conn, "response:script:exec", request.ID, false, "script bridge is unavailable", "", "", nil)
	}
	stdout, stderr, code, err := rt.ScriptBridge.Exec(request.Runner, request.File, request.Args, requestTimeout(request.Timeout))
	ok := err == nil
	exitCode := code
	if !ok && exitCode == 0 {
		exitCode = -1
	}
	rt.logEvent("script_exec", map[string]any{
		"runner":    request.Runner,
		"file":      request.File,
		"ok":        ok,
		"exit_code": exitCode,
	})
	if err != nil {
		return WriteExecResponse(conn, "response:script:exec", request.ID, false, err.Error(), stdout, stderr, intPtr(code))
	}
	return WriteExecResponse(conn, "response:script:exec", request.ID, true, "", stdout, stderr, intPtr(code))
}

func (rt *Runtime) handleShellRequest(conn *WSConnection, request WSMessage) error {
	if rt == nil {
		return WriteErrorResponse(conn, request.ID, "runtime is required")
	}
	if !rt.Capabilities.Shell {
		return WriteExecResponse(conn, "response:shell:exec", request.ID, false, "shell capability is disabled", "", "", nil)
	}
	if rt.ShellBridge == nil {
		return WriteExecResponse(conn, "response:shell:exec", request.ID, false, "shell bridge is unavailable", "", "", nil)
	}
	stdout, stderr, code, err := rt.ShellBridge.Exec(request.Cmd, request.Args, requestTimeout(request.Timeout))
	ok := err == nil
	exitCode := code
	if !ok && exitCode == 0 {
		exitCode = -1
	}
	rt.logEvent("shell_exec", map[string]any{
		"cmd":       request.Cmd,
		"ok":        ok,
		"exit_code": exitCode,
	})
	if err != nil {
		return WriteExecResponse(conn, "response:shell:exec", request.ID, false, err.Error(), stdout, stderr, intPtr(code))
	}
	return WriteExecResponse(conn, "response:shell:exec", request.ID, true, "", stdout, stderr, intPtr(code))
}

func (rt *Runtime) pushFSChanged(path string) error {
	rt.logEvent("fs_changed", map[string]any{"path": path})
	return pushWSMessage(rt, WSMessage{Event: "file:changed", Path: path})
}

func pushWSMessage(rt *Runtime, message WSMessage) error {
	if rt == nil {
		return nil
	}
	var firstErr error
	for _, conn := range rt.connectionSnapshot() {
		if err := WriteWSMessage(conn, message); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func requestTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func boolPtr(v bool) *bool {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func (rt *Runtime) handleWSAuth(conn *WSConnection, request WSMessage) error {
	if rt == nil || conn == nil {
		return nil
	}
	if conn.authenticated {
		_ = WriteErrorResponse(conn, request.ID, "already authenticated")
		return nil
	}
	token := request.dataString()
	if token == "" {
		rt.logEvent("error", map[string]any{"message": "ws_auth: missing token"})
		return WriteErrorResponse(conn, request.ID, "missing authentication token")
	}
	if rt.wsNonce == "" || token != rt.wsNonce {
		rt.logEvent("error", map[string]any{"message": "ws_auth: invalid token"})
		return WriteErrorResponse(conn, request.ID, "invalid authentication token")
	}
	conn.authenticated = true
	rt.logEvent("ws_auth", map[string]any{"message": "connection authenticated"})
	return WriteWSMessage(conn, WSMessage{
		Event: "ws_auth_response",
		ID:    request.ID,
		Ok:    boolPtr(true),
	})
}
