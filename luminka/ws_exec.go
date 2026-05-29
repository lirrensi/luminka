// FILE: luminka/ws_exec.go
// PURPOSE: Route streaming script and shell websocket events.
// OWNS: Script and shell streaming request dispatch and capability gating.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

func (rt *Runtime) handleScriptStreamRequest(conn *WSConnection, request WSMessage) error {
	if rt == nil {
		return WriteErrorResponse(conn, request.ID, "runtime is required")
	}
	if !rt.Capabilities.Scripts {
		return WriteExecResponse(conn, "response:script:exec", request.ID, false, "script capability is disabled", "", "", nil)
	}
	if rt.ScriptBridge == nil {
		return WriteExecResponse(conn, "response:script:exec", request.ID, false, "script bridge is unavailable", "", "", nil)
	}
	if err := rt.ScriptBridge.ExecStream(rt, conn, request.ID, request.Runner, request.File, request.Args, requestTimeout(request.Timeout)); err != nil {
		rt.logEvent("script_exec", map[string]any{
			"runner": request.Runner,
			"file":   request.File,
			"ok":     false,
			"stream": true,
		})
		return WriteExecResponse(conn, "response:script:exec", request.ID, false, err.Error(), "", "", nil)
	}
	rt.logEvent("script_exec", map[string]any{
		"runner": request.Runner,
		"file":   request.File,
		"ok":     true,
		"stream": true,
	})
	return nil
}

func (rt *Runtime) handleShellStreamRequest(conn *WSConnection, request WSMessage) error {
	if rt == nil {
		return WriteErrorResponse(conn, request.ID, "runtime is required")
	}
	if !rt.Capabilities.Shell {
		return WriteExecResponse(conn, "response:shell:exec", request.ID, false, "shell capability is disabled", "", "", nil)
	}
	if rt.ShellBridge == nil {
		return WriteExecResponse(conn, "response:shell:exec", request.ID, false, "shell bridge is unavailable", "", "", nil)
	}
	if err := rt.ShellBridge.ExecStream(rt, conn, request.ID, request.Cmd, request.Args, requestTimeout(request.Timeout)); err != nil {
		rt.logEvent("shell_exec", map[string]any{
			"cmd":    request.Cmd,
			"ok":     false,
			"stream": true,
		})
		return WriteExecResponse(conn, "response:shell:exec", request.ID, false, err.Error(), "", "", nil)
	}
	rt.logEvent("shell_exec", map[string]any{
		"cmd":    request.Cmd,
		"ok":     true,
		"stream": true,
	})
	return nil
}
