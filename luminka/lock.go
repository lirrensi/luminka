// FILE: luminka/lock.go
// PURPOSE: Resolve the runtime root and manage single-instance lock files.
// OWNS: Root resolution, lock parsing, lock creation, stale lock recovery, and cleanup.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_phase1_runtime_2026-03-30.md, agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func resolveRootDirectory(root string, policy RootPolicy) (string, error) {
	if root != "" {
		return resolveAbsoluteRoot(root)
	}
	switch policy {
	case RootPolicyDetached:
		return resolveWorkingDir()
	case "", RootPolicyPortable:
		return resolveExecutableDir()
	default:
		return "", fmt.Errorf("unsupported root policy %q", policy)
	}
}

func resolveExecutableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	return filepath.Dir(exe), nil
}

func resolveWorkingDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return resolveAbsoluteRoot(wd)
}

func resolveAbsoluteRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return abs, nil
}

func lockFilePath(root, name string) string {
	return filepath.Join(root, fmt.Sprintf("%s.lock", name))
}

// acquireInstanceLock reads the lock file for an existing instance without creating one.
// Lock file creation is deferred to writeInstanceRecord after the server port is known.
func acquireInstanceLock(root, name string) (*lockState, error) {
	path := lockFilePath(root, name)

	record, readErr := readLockRecord(path)
	if readErr != nil {
		// No valid lock exists — caller will create one after server starts.
		return nil, nil
	}

	if !processAlive(record.PID) {
		// Stale lock — remove and signal caller to create a new one.
		_ = os.Remove(path)
		return nil, nil
	}

	// Existing instance is alive.
	if record.Port > 0 {
		if localhostPortReachable(record.Port, 250*time.Millisecond) {
			return &lockState{path: path, record: *record, reused: true}, nil
		}
		// Port is not reachable — stale lock (process alive but port dead).
		_ = os.Remove(path)
		return nil, nil
	}

	// Process alive but port not yet known (still starting up) — treat as existing.
	return &lockState{path: path, record: *record, reused: true}, nil
}

func localhostPortReachable(port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

type instanceRecord struct {
	PID    int                  `json:"pid"`
	Port   int                  `json:"port"`
	Mode   Mode                 `json:"mode,omitempty"`
	Window instanceWindowRecord `json:"window,omitempty"`
}

type instanceWindowRecord struct {
	Platform string `json:"platform,omitempty"`
	ID       string `json:"id,omitempty"`
}

func readLockRecord(path string) (*instanceRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var record instanceRecord
	if err := json.Unmarshal(data, &record); err == nil && record.PID > 0 {
		return &record, nil
	}
	pid, port, parseErr := parseLockRecord(string(data))
	if parseErr != nil {
		return nil, parseErr
	}
	return &instanceRecord{PID: pid, Port: port}, nil
}

func parseLockRecord(raw string) (int, int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, errors.New("empty lock record")
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid lock record %q", raw)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	port, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, err
	}
	return pid, port, nil
}

func writeInstanceRecord(path string, record instanceRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func removeLockFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
