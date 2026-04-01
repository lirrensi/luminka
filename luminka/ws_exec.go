// FILE: luminka/ws_exec.go
// PURPOSE: Route streaming script and shell websocket events.
// OWNS: Script and shell streaming request dispatch and capability gating.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

func (rt *Runtime) handleScriptStreamRequest(conn *wsConnection, request wsMessage) error {
	if rt == nil {
		return writeErrorResponse(conn, request.ID, "runtime is required")
	}
	if !rt.Capabilities.Scripts {
		return writeExecResponse(conn, "script_response", request.ID, false, "script capability is disabled", "", "", nil)
	}
	if rt.ScriptBridge == nil {
		return writeExecResponse(conn, "script_response", request.ID, false, "script bridge is unavailable", "", "", nil)
	}
	if err := rt.ScriptBridge.ExecStream(rt, conn, request.ID, request.Runner, request.File, request.Args, requestTimeout(request.Timeout)); err != nil {
		return writeExecResponse(conn, "script_response", request.ID, false, err.Error(), "", "", nil)
	}
	return nil
}

func (rt *Runtime) handleShellStreamRequest(conn *wsConnection, request wsMessage) error {
	if rt == nil {
		return writeErrorResponse(conn, request.ID, "runtime is required")
	}
	if !rt.Capabilities.Shell {
		return writeExecResponse(conn, "shell_response", request.ID, false, "shell capability is disabled", "", "", nil)
	}
	if rt.ShellBridge == nil {
		return writeExecResponse(conn, "shell_response", request.ID, false, "shell bridge is unavailable", "", "", nil)
	}
	if err := rt.ShellBridge.ExecStream(rt, conn, request.ID, request.Cmd, request.Args, requestTimeout(request.Timeout)); err != nil {
		return writeExecResponse(conn, "shell_response", request.ID, false, err.Error(), "", "", nil)
	}
	return nil
}
