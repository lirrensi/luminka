// FILE: luminka/ws.go
// PURPOSE: Serve the canonical Phase 2 websocket protocol and capability dispatch.
// OWNS: Connection tracking, request routing, capability gating, and push notifications.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md

package luminka

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type wsMessage struct {
	Event        string          `json:"event"`
	ID           json.RawMessage `json:"id,omitempty"`
	Ok           *bool           `json:"ok,omitempty"`
	Error        string          `json:"error,omitempty"`
	Path         string          `json:"path,omitempty"`
	Data         string          `json:"data,omitempty"`
	Files        []string        `json:"files,omitempty"`
	Exists       *bool           `json:"exists,omitempty"`
	Runner       string          `json:"runner,omitempty"`
	File         string          `json:"file,omitempty"`
	Cmd          string          `json:"cmd,omitempty"`
	Args         []string        `json:"args,omitempty"`
	Timeout      int             `json:"timeout,omitempty"`
	Stdout       string          `json:"stdout,omitempty"`
	Stderr       string          `json:"stderr,omitempty"`
	Code         *int            `json:"code,omitempty"`
	Name         string          `json:"name,omitempty"`
	Mode         Mode            `json:"mode,omitempty"`
	Root         string          `json:"root,omitempty"`
	StreamID     string          `json:"stream_id,omitempty"`
	Seq          uint64          `json:"seq,omitempty"`
	Lane         string          `json:"lane,omitempty"`
	EOF          bool            `json:"eof,omitempty"`
	Capabilities capabilityState `json:"capabilities,omitempty"`
}

type websocketConn interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(int, []byte) error
	Close() error
}

type wsConnection struct {
	conn    websocketConn
	writeMu sync.Mutex
}

func (m wsMessage) MarshalJSON() ([]byte, error) {
	type wire struct {
		Event        string           `json:"event"`
		ID           json.RawMessage  `json:"id,omitempty"`
		Ok           *bool            `json:"ok,omitempty"`
		Error        string           `json:"error,omitempty"`
		Path         string           `json:"path,omitempty"`
		Data         string           `json:"data,omitempty"`
		Files        []string         `json:"files,omitempty"`
		Exists       *bool            `json:"exists,omitempty"`
		Runner       string           `json:"runner,omitempty"`
		File         string           `json:"file,omitempty"`
		Cmd          string           `json:"cmd,omitempty"`
		Args         []string         `json:"args,omitempty"`
		Timeout      int              `json:"timeout,omitempty"`
		Stdout       string           `json:"stdout,omitempty"`
		Stderr       string           `json:"stderr,omitempty"`
		Code         *int             `json:"code,omitempty"`
		Name         string           `json:"name,omitempty"`
		Mode         Mode             `json:"mode,omitempty"`
		Root         string           `json:"root,omitempty"`
		StreamID     string           `json:"stream_id,omitempty"`
		Seq          uint64           `json:"seq,omitempty"`
		Lane         string           `json:"lane,omitempty"`
		EOF          bool             `json:"eof,omitempty"`
		Capabilities *capabilityState `json:"capabilities,omitempty"`
	}
	out := wire{
		Event:    m.Event,
		ID:       m.ID,
		Ok:       m.Ok,
		Error:    m.Error,
		Path:     m.Path,
		Data:     m.Data,
		Files:    m.Files,
		Exists:   m.Exists,
		Runner:   m.Runner,
		File:     m.File,
		Cmd:      m.Cmd,
		Args:     m.Args,
		Timeout:  m.Timeout,
		Stdout:   m.Stdout,
		Stderr:   m.Stderr,
		Code:     m.Code,
		Name:     m.Name,
		Mode:     m.Mode,
		Root:     m.Root,
		StreamID: m.StreamID,
		Seq:      m.Seq,
		Lane:     m.Lane,
		EOF:      m.EOF,
	}
	if m.Event == "app_info" || m.Capabilities != (capabilityState{}) {
		caps := m.Capabilities
		out.Capabilities = &caps
	}
	return json.Marshal(out)
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

	wsConn := rt.registerConnection(conn)
	defer func() {
		rt.unregisterConnection(wsConn)
		_ = conn.Close()
	}()
	rt.handleWebSocketSession(wsConn)
}

func (rt *Runtime) handleWebSocketSession(wsConn *wsConnection) {
	if rt == nil || wsConn == nil {
		return
	}

	for {
		msgType, request, payload, err := readWSFrame(wsConn)
		if err != nil {
			if msgType == websocket.BinaryMessage {
				_ = writeErrorResponse(wsConn, nil, err.Error())
				continue
			}
			return
		}
		if msgType == websocket.TextMessage {
			_ = writeErrorResponse(wsConn, nil, "websocket text frames are not supported")
			continue
		}
		if request.Event == "" {
			_ = writeErrorResponse(wsConn, request.ID, "message event is required")
			continue
		}

		switch request.Event {
		case "app_info":
			_ = writeWSMessage(wsConn, wsMessage{
				Event:        "app_info",
				ID:           request.ID,
				Ok:           boolPtr(true),
				Name:         rt.Name,
				Mode:         rt.Mode,
				Root:         rt.Root,
				Capabilities: rt.Capabilities,
			})
		case "fs_read_text", "fs_write_text", "fs_list", "fs_delete", "fs_exists", "fs_watch", "fs_unwatch", "fs_open_read", "fs_open_write", "stream_chunk", "stream_close":
			_ = rt.handleFilesystemRequest(wsConn, request, payload)
		case "script_exec":
			_ = rt.handleScriptRequest(wsConn, request)
		case "script_exec_stream":
			_ = rt.handleScriptStreamRequest(wsConn, request)
		case "shell_exec":
			_ = rt.handleShellRequest(wsConn, request)
		case "shell_exec_stream":
			_ = rt.handleShellStreamRequest(wsConn, request)
		default:
			_ = writeErrorResponse(wsConn, request.ID, fmt.Sprintf("unknown event %q", request.Event))
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

func (rt *Runtime) registerConnection(conn websocketConn) *wsConnection {
	if rt == nil || conn == nil {
		return nil
	}
	wsConn := &wsConnection{conn: conn}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.connections == nil {
		rt.connections = make(map[*wsConnection]struct{})
	}
	rt.connections[wsConn] = struct{}{}
	if rt.idleTimer != nil {
		rt.idleTimer.Stop()
		rt.idleTimer = nil
	}
	return wsConn
}

func (rt *Runtime) unregisterConnection(conn *wsConnection) {
	if rt == nil || conn == nil {
		return
	}
	if rt.streams != nil {
		rt.streams.closeConnection(conn)
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.connections, conn)
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

func (rt *Runtime) connectionSnapshot() []*wsConnection {
	if rt == nil {
		return nil
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.connections) == 0 {
		return nil
	}
	out := make([]*wsConnection, 0, len(rt.connections))
	for conn := range rt.connections {
		out = append(out, conn)
	}
	return out
}

func (rt *Runtime) handleScriptRequest(conn *wsConnection, request wsMessage) error {
	if rt == nil {
		return writeErrorResponse(conn, request.ID, "runtime is required")
	}
	if !rt.Capabilities.Scripts {
		return writeExecResponse(conn, "script_response", request.ID, false, "script capability is disabled", "", "", nil)
	}
	if rt.ScriptBridge == nil {
		return writeExecResponse(conn, "script_response", request.ID, false, "script bridge is unavailable", "", "", nil)
	}
	stdout, stderr, code, err := rt.ScriptBridge.Exec(request.Runner, request.File, request.Args, requestTimeout(request.Timeout))
	if err != nil {
		return writeExecResponse(conn, "script_response", request.ID, false, err.Error(), stdout, stderr, intPtr(code))
	}
	return writeExecResponse(conn, "script_response", request.ID, true, "", stdout, stderr, intPtr(code))
}

func (rt *Runtime) handleShellRequest(conn *wsConnection, request wsMessage) error {
	if rt == nil {
		return writeErrorResponse(conn, request.ID, "runtime is required")
	}
	if !rt.Capabilities.Shell {
		return writeExecResponse(conn, "shell_response", request.ID, false, "shell capability is disabled", "", "", nil)
	}
	if rt.ShellBridge == nil {
		return writeExecResponse(conn, "shell_response", request.ID, false, "shell bridge is unavailable", "", "", nil)
	}
	stdout, stderr, code, err := rt.ShellBridge.Exec(request.Cmd, request.Args, requestTimeout(request.Timeout))
	if err != nil {
		return writeExecResponse(conn, "shell_response", request.ID, false, err.Error(), stdout, stderr, intPtr(code))
	}
	return writeExecResponse(conn, "shell_response", request.ID, true, "", stdout, stderr, intPtr(code))
}

func (rt *Runtime) pushFSChanged(path string) error {
	return pushWSMessage(rt, wsMessage{Event: "fs_changed", Path: path})
}

func pushWSMessage(rt *Runtime, message wsMessage) error {
	if rt == nil {
		return nil
	}
	var firstErr error
	for _, conn := range rt.connectionSnapshot() {
		if err := writeWSMessage(conn, message); err != nil {
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
