// FILE: luminka/headless.go
// PURPOSE: Wait for process shutdown in headless launches without opening UI shells.
// OWNS: Headless runtime shutdown waiting.
// EXPORTS: runHeadless
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import "errors"

func runHeadless(rt *Runtime) error {
	if rt == nil {
		return errors.New("runtime is required")
	}
	return rt.waitForShutdown()
}
