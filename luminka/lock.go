// FILE: luminka/lock.go
// PURPOSE: Resolve the runtime root and manage single-instance lock files.
// OWNS: Root resolution, lock parsing, lock creation, stale lock recovery, and cleanup.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_phase1_runtime_2026-03-30.md

package luminka

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func resolveRootDirectory(root string) (string, error) {
	if root == "" {
		return resolveExecutableDir()
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return abs, nil
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

func lockFilePath(root, name string) string {
	return filepath.Join(root, fmt.Sprintf("%s.lock", name))
}

func acquireInstanceLock(root, name string) (*lockState, error) {
	path := lockFilePath(root, name)
	pid := os.Getpid()

	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			if _, writeErr := fmt.Fprintf(f, "%d:0", pid); writeErr != nil {
				_ = f.Close()
				_ = os.Remove(path)
				return nil, writeErr
			}
			if closeErr := f.Close(); closeErr != nil {
				_ = os.Remove(path)
				return nil, closeErr
			}
			return &lockState{path: path, pid: pid, owned: true}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}

		record, readErr := readLockRecord(path)
		if readErr != nil {
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				return nil, readErr
			}
			continue
		}

		if processAlive(record.pid) {
			if record.port > 0 {
				return &lockState{path: path, pid: record.pid, port: record.port, reused: true}, nil
			}
			if record.port == 0 {
				return &lockState{path: path, pid: record.pid, port: 0, reused: true}, nil
			}
		}

		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, removeErr
		}
	}
}

type lockRecord struct {
	pid  int
	port int
}

func readLockRecord(path string) (*lockRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pid, port, err := parseLockRecord(string(data))
	if err != nil {
		return nil, err
	}
	return &lockRecord{pid: pid, port: port}, nil
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

func writeLockPort(path string, pid, port int) error {
	return os.WriteFile(path, []byte(fmt.Sprintf("%d:%d", pid, port)), 0o644)
}

func removeLockFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
