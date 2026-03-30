//go:build scripts

// FILE: luminka/script_test.go
// PURPOSE: Verify script bridge resolution for external files and embedded internal scripts.
// OWNS: Script execution tests for relative-path and internal bundle resolution.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_luminka_internal_scripts_2026-03-30.md

package luminka

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestScriptBridgeExecutesExternalAndInternalScripts(t *testing.T) {
	root := t.TempDir()
	runner := buildScriptReaderRunner(t, root)
	if err := os.WriteFile(filepath.Join(root, "external.txt"), []byte("external-ok"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	sb := &ScriptBridge{
		root:           root,
		defaultTimeout: time.Second,
		scriptAssets:   fstest.MapFS{"scripts/internal.txt": &fstest.MapFile{Data: []byte("internal-ok")}},
	}

	stdout, stderr, code, err := sb.Exec(runner, "external.txt", nil, 0)
	if err != nil {
		t.Fatalf("external Exec() error = %v", err)
	}
	if code != 0 || stdout != "external-ok" || stderr != "" {
		t.Fatalf("external Exec() = stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}

	stdout, stderr, code, err = sb.Exec(runner, "internal:scripts/internal.txt", nil, 0)
	if err != nil {
		t.Fatalf("internal Exec() error = %v", err)
	}
	if code != 0 || stdout != "internal-ok" || stderr != "" {
		t.Fatalf("internal Exec() = stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func TestScriptBridgeRejectsMissingInternalBundleAndFile(t *testing.T) {
	root := t.TempDir()
	runner := buildScriptReaderRunner(t, root)

	missingBundle := &ScriptBridge{root: root, defaultTimeout: time.Second}
	if _, _, _, err := missingBundle.Exec(runner, "internal:scripts/internal.txt", nil, 0); err == nil || !strings.Contains(err.Error(), "embedded script bundle is required") {
		t.Fatalf("missing bundle error = %v, want embedded script bundle is required", err)
	}

	withBundle := &ScriptBridge{
		root:           root,
		defaultTimeout: time.Second,
		scriptAssets:   fstest.MapFS{"scripts/other.txt": &fstest.MapFile{Data: []byte("ok")}},
	}
	if _, _, _, err := withBundle.Exec(runner, "internal:scripts/missing.txt", nil, 0); err == nil || !strings.Contains(err.Error(), "embedded script not found") {
		t.Fatalf("missing internal file error = %v, want embedded script not found", err)
	}

	if _, _, _, err := withBundle.Exec(runner, "internal:", nil, 0); err == nil || !strings.Contains(err.Error(), "invalid internal selector") {
		t.Fatalf("invalid selector error = %v, want invalid internal selector", err)
	}
	if _, _, _, err := withBundle.Exec(runner, "internal:../escape.txt", nil, 0); err == nil || !strings.Contains(err.Error(), "invalid internal selector") {
		t.Fatalf("escape selector error = %v, want invalid internal selector", err)
	}
}

func buildScriptReaderRunner(t *testing.T, dir string) string {
	t.Helper()
	goMod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module runner\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	source := filepath.Join(dir, "runner.go")
	if err := os.WriteFile(source, []byte(scriptReaderRunnerSource), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	out := filepath.Join(dir, "runner"+exeSuffix())
	cmd := exec.Command("go", "build", "-o", out, source)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build runner failed: %v\n%s", err, string(output))
	}
	return out
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

const scriptReaderRunnerSource = `package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(3)
	}
	fmt.Print(string(data))
}`
