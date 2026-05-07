// FILE: luminka/log.go
// PURPOSE: Append-only JSON Lines event logging to luminka.log in the app root, gated by the --logs flag.
// OWNS: Runtime event log format, log file path, and silent-write semantics.
// EXPORTS: none
// DOCS: agent_chat/plan_fsnotify_log_recipes_2026-05-08.md

package luminka

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// logEvent appends a timestamped JSON Lines entry to <root>/luminka.log when logging is enabled.
// Log write failures are silently ignored — logging is a best-effort convenience, not a contract.
func (rt *Runtime) logEvent(event string, fields map[string]any) {
	if rt == nil || !rt.Logs {
		return
	}

	logPath := filepath.Join(rt.Root, "luminka.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	entry := map[string]any{
		"time":  time.Now().UTC().Format(time.RFC3339),
		"event": event,
	}
	for k, v := range fields {
		entry[k] = v
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = f.Write(append(data, '\n'))
}
