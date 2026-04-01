//go:build shell

// FILE: luminka/shell_test.go
// PURPOSE: Verify streaming shell execution over websocket transport.
// OWNS: Shell stdout chunk streaming and final completion coverage.
// EXPORTS: TestShellBridgeExecStreamStreamsStdout
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestShellBridgeExecStreamStreamsStdout(t *testing.T) {
	root := t.TempDir()
	sb := &ShellBridge{root: root, defaultTimeout: time.Second}
	rt := &Runtime{streams: newStreamRegistry()}
	conn := newFakeWebSocketConn()
	wsConn := &wsConnection{conn: conn}

	cmd, args, want := shellEchoCommand("shell-ok")
	if err := sb.ExecStream(rt, wsConn, json.RawMessage(`"h1"`), cmd, args, 0); err != nil {
		t.Fatalf("ExecStream() error = %v", err)
	}

	var stdout strings.Builder
	for {
		header, payload := mustReadWSFrame(t, conn)
		switch header["event"] {
		case "stream_chunk":
			if lane, _ := header["lane"].(string); lane != "stdout" {
				t.Fatalf("lane = %q, want stdout", lane)
			}
			stdout.Write(payload)
		case "shell_response":
			assertWSOK(t, header, "shell_response", "h1")
			if code, _ := header["code"].(float64); code != 0 {
				t.Fatalf("code = %v, want 0", header["code"])
			}
			if got := strings.TrimSpace(stdout.String()); got != want {
				t.Fatalf("stdout = %q, want %q", got, want)
			}
			return
		default:
			t.Fatalf("unexpected event %q", header["event"])
		}
	}
}

func TestShellBridgeExecStreamReportsNonZeroExitCode(t *testing.T) {
	root := t.TempDir()
	sb := &ShellBridge{root: root, defaultTimeout: time.Second}
	rt := &Runtime{streams: newStreamRegistry()}
	conn := newFakeWebSocketConn()
	wsConn := &wsConnection{conn: conn}

	cmd, args := shellExitCommand()
	if err := sb.ExecStream(rt, wsConn, json.RawMessage(`"h2"`), cmd, args, 0); err != nil {
		t.Fatalf("ExecStream() error = %v", err)
	}

	for {
		header, _ := mustReadWSFrame(t, conn)
		switch header["event"] {
		case "stream_chunk":
			// drain stdout/stderr chunks
		case "shell_response":
			if ok, _ := header["ok"].(bool); ok {
				t.Fatal("shell_response ok = true, want false for non-zero exit")
			}
			if code, _ := header["code"].(float64); code != 9 {
				t.Fatalf("code = %v, want 9", header["code"])
			}
			return
		default:
			t.Fatalf("unexpected event %q", header["event"])
		}
	}
}

func shellEchoCommand(text string) (string, []string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo", text}, text
	}
	return "sh", []string{"-c", "printf %s " + text}, text
}

func shellExitCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo shell-fail & exit /b 9"}
	}
	return "sh", []string{"-c", "printf shell-fail; exit 9"}
}
