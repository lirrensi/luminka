// FILE: luminka/runtime_test.go
// PURPOSE: Verify runtime configuration defaults, lock ownership semantics, and second-launch decisions.
// OWNS: Deterministic tests for runtime prep, startup lock behavior, and reused-instance handling.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_luminka_runtime_safety_2026-03-30.md

package luminka

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func TestNormalizeConfigDefaultsWindowAndTimeouts(t *testing.T) {
	got := normalizeConfig(Config{Name: "demo"})

	if got.Mode != ModeBrowser {
		t.Fatalf("Mode = %s, want %s", got.Mode, ModeBrowser)
	}
	if got.WindowTitle != "demo" {
		t.Fatalf("WindowTitle = %q, want demo", got.WindowTitle)
	}
	if got.WindowWidth != 1280 || got.WindowHeight != 800 {
		t.Fatalf("window size = %dx%d, want 1280x800", got.WindowWidth, got.WindowHeight)
	}
	if got.Idle != defaultIdleTimeout {
		t.Fatalf("Idle = %v, want %v", got.Idle, defaultIdleTimeout)
	}
	if got.ExecTimeout != 30*time.Second {
		t.Fatalf("ExecTimeout = %v, want 30s", got.ExecTimeout)
	}
	if got.WindowResizable {
		t.Fatal("WindowResizable defaulted to true, want false")
	}
	if got.WindowDebug {
		t.Fatal("WindowDebug defaulted to true, want false")
	}
}

func TestPrepareRuntimeResolvesCapabilitiesAndWindowFields(t *testing.T) {
	root := t.TempDir()
	rt, existing, err := prepareRuntime(normalizeConfig(Config{
		Name:            "prepare-runtime-test",
		Mode:            ModeWebview,
		Root:            root,
		WindowTitle:     "custom-title",
		WindowWidth:     1440,
		WindowHeight:    900,
		WindowResizable: true,
		WindowDebug:     true,
		DisableFS:       true,
		EnableScripts:   true,
		EnableShell:     true,
		ExecTimeout:     5 * time.Second,
		Assets:          fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}},
	}))
	if err != nil {
		t.Fatalf("prepareRuntime() error = %v", err)
	}
	if existing != nil {
		t.Fatalf("prepareRuntime() existing = %#v, want nil", existing)
	}
	t.Cleanup(func() { _ = rt.cleanup() })

	if rt.Mode != ModeWebview {
		t.Fatalf("Mode = %s, want %s", rt.Mode, ModeWebview)
	}
	if rt.WindowTitle != "custom-title" || rt.WindowWidth != 1440 || rt.WindowHeight != 900 {
		t.Fatalf("window config copied incorrectly: title=%q width=%d height=%d", rt.WindowTitle, rt.WindowWidth, rt.WindowHeight)
	}
	if !rt.WindowResizable || !rt.WindowDebug {
		t.Fatalf("window flags copied incorrectly: resizable=%v debug=%v", rt.WindowResizable, rt.WindowDebug)
	}
	if rt.Capabilities.FS {
		t.Fatal("Capabilities.FS = true, want false when DisableFS is set")
	}
	if rt.Capabilities.Scripts != scriptSupportAvailable() {
		t.Fatalf("Capabilities.Scripts = %v, want %v", rt.Capabilities.Scripts, scriptSupportAvailable())
	}
	if rt.Capabilities.Shell != shellSupportAvailable() {
		t.Fatalf("Capabilities.Shell = %v, want %v", rt.Capabilities.Shell, shellSupportAvailable())
	}
}

func TestResolveRootDirectoryPortableUsesExecutableDir(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	want := filepath.Dir(exe)

	got, err := resolveRootDirectory("", RootPolicyPortable)
	if err != nil {
		t.Fatalf("resolveRootDirectory() error = %v", err)
	}
	if got != want {
		t.Fatalf("portable root = %q, want %q", got, want)
	}
}

func TestResolveRootDirectoryDetachedUsesWorkingDir(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })

	got, err := resolveRootDirectory("", RootPolicyDetached)
	if err != nil {
		t.Fatalf("resolveRootDirectory() error = %v", err)
	}
	if got != tempDir {
		t.Fatalf("detached root = %q, want %q", got, tempDir)
	}
}

func TestApplyLaunchOverridesPrefersExplicitRoot(t *testing.T) {
	cfg := normalizeConfig(Config{Root: "config-root", RootPolicy: RootPolicyPortable})
	got := applyLaunchOverrides(cfg, launchOptions{Root: "launch-root", RootPolicy: RootPolicyDetached, Headless: true})

	if got.Root != "launch-root" {
		t.Fatalf("Root = %q, want launch-root", got.Root)
	}
	if got.RootPolicy != RootPolicyDetached {
		t.Fatalf("RootPolicy = %q, want detached", got.RootPolicy)
	}
	if !got.Headless {
		t.Fatal("Headless = false, want true")
	}
}

func TestRuntimeLaunchModeForHeadlessBrowserAndWebview(t *testing.T) {
	if got := runtimeLaunchModeFor(&Runtime{Mode: ModeBrowser, Headless: true}); got != runtimeLaunchHeadless {
		t.Fatalf("browser headless launch mode = %q, want %q", got, runtimeLaunchHeadless)
	}
	if got := runtimeLaunchModeFor(&Runtime{Mode: ModeWebview, Headless: true}); got != runtimeLaunchHeadless {
		t.Fatalf("webview headless launch mode = %q, want %q", got, runtimeLaunchHeadless)
	}
}

func TestAcquireInstanceLockCreatesFreshPIDZeroRecord(t *testing.T) {
	root := t.TempDir()
	state, err := acquireInstanceLock(root, "runtime-lock-fresh")
	if err != nil {
		t.Fatalf("acquireInstanceLock() error = %v", err)
	}
	t.Cleanup(func() { _ = removeLockFile(state.path) })

	if !state.owned || state.reused {
		t.Fatalf("lock state = %#v, want owned fresh lock", state)
	}
	if state.pid != os.Getpid() {
		t.Fatalf("pid = %d, want %d", state.pid, os.Getpid())
	}
	if state.port != 0 {
		t.Fatalf("port = %d, want 0", state.port)
	}

	data, err := os.ReadFile(state.path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := fmt.Sprintf("%d:0", os.Getpid())
	if string(data) != want {
		t.Fatalf("lock contents = %q, want %q", string(data), want)
	}
}

func TestAcquireInstanceLockReusesLivePIDZeroRecord(t *testing.T) {
	root := t.TempDir()
	first, err := acquireInstanceLock(root, "runtime-lock-live")
	if err != nil {
		t.Fatalf("first acquireInstanceLock() error = %v", err)
	}
	t.Cleanup(func() { _ = removeLockFile(first.path) })

	second, err := acquireInstanceLock(root, "runtime-lock-live")
	if err != nil {
		t.Fatalf("second acquireInstanceLock() error = %v", err)
	}

	if second == nil || !second.reused || second.owned {
		t.Fatalf("second lock state = %#v, want reused live lock", second)
	}
	if second.pid != os.Getpid() {
		t.Fatalf("pid = %d, want %d", second.pid, os.Getpid())
	}
	if second.port != 0 {
		t.Fatalf("port = %d, want 0", second.port)
	}

	data, err := os.ReadFile(first.path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := fmt.Sprintf("%d:0", os.Getpid())
	if string(data) != want {
		t.Fatalf("lock contents = %q, want %q", string(data), want)
	}
}

func TestAcquireInstanceLockRecoversStalePIDZeroRecord(t *testing.T) {
	const stalePID = 999999
	if processAlive(stalePID) {
		t.Skipf("pid %d is unexpectedly alive on this system", stalePID)
	}

	root := t.TempDir()
	path := lockFilePath(root, "runtime-lock-stale")
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d:0", stalePID)), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	state, err := acquireInstanceLock(root, "runtime-lock-stale")
	if err != nil {
		t.Fatalf("acquireInstanceLock() error = %v", err)
	}
	t.Cleanup(func() { _ = removeLockFile(state.path) })

	if !state.owned || state.reused {
		t.Fatalf("lock state = %#v, want fresh owned lock after stale recovery", state)
	}
	if state.pid != os.Getpid() {
		t.Fatalf("pid = %d, want %d", state.pid, os.Getpid())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := fmt.Sprintf("%d:0", os.Getpid())
	if string(data) != want {
		t.Fatalf("lock contents = %q, want %q", string(data), want)
	}
}

func TestAcquireInstanceLockReusesLiveReachablePortRecord(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-acceptDone
	})

	root := t.TempDir()
	path := lockFilePath(root, "runtime-lock-live-port")
	port := listener.Addr().(*net.TCPAddr).Port
	if !localhostPortReachable(port, 250*time.Millisecond) {
		t.Skipf("localhost loopback is unavailable in this environment; localhostPortReachable(%d) returned false", port)
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d:%d", os.Getpid(), port)), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	state, err := acquireInstanceLock(root, "runtime-lock-live-port")
	if err != nil {
		t.Fatalf("acquireInstanceLock() error = %v", err)
	}

	if state == nil || !state.reused || state.owned {
		t.Fatalf("lock state = %#v, want reused reachable-port lock", state)
	}
	if state.pid != os.Getpid() {
		t.Fatalf("pid = %d, want %d", state.pid, os.Getpid())
	}
	if state.port != port {
		t.Fatalf("port = %d, want %d", state.port, port)
	}
}

func TestAcquireInstanceLockRecoversStaleClosedPortRecord(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	root := t.TempDir()
	path := lockFilePath(root, "runtime-lock-closed-port")
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d:%d", os.Getpid(), port)), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	state, err := acquireInstanceLock(root, "runtime-lock-closed-port")
	if err != nil {
		t.Fatalf("acquireInstanceLock() error = %v", err)
	}
	t.Cleanup(func() { _ = removeLockFile(state.path) })

	if !state.owned || state.reused {
		t.Fatalf("lock state = %#v, want fresh owned lock after closed-port recovery", state)
	}
	if state.pid != os.Getpid() {
		t.Fatalf("pid = %d, want %d", state.pid, os.Getpid())
	}
	if state.port != 0 {
		t.Fatalf("port = %d, want 0", state.port)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := fmt.Sprintf("%d:0", os.Getpid())
	if string(data) != want {
		t.Fatalf("lock contents = %q, want %q", string(data), want)
	}
}

func TestDecideExistingInstanceActionOpensBrowserForRunningBrowserInstance(t *testing.T) {
	action := decideExistingInstanceAction(Config{Mode: ModeBrowser}, &lockState{pid: os.Getpid(), port: 43123, reused: true})

	if action.continueStartup {
		t.Fatal("continueStartup = true, want false")
	}
	if !action.openBrowser {
		t.Fatal("openBrowser = false, want true")
	}
	if action.browserURL != localURL(43123) {
		t.Fatalf("browserURL = %q, want %q", action.browserURL, localURL(43123))
	}
}

func TestDecideExistingInstanceActionContinuesStartupWithoutExistingLock(t *testing.T) {
	action := decideExistingInstanceAction(Config{Mode: ModeBrowser}, nil)

	if !action.continueStartup {
		t.Fatal("continueStartup = false, want true")
	}
	if action.openBrowser {
		t.Fatal("openBrowser = true, want false")
	}
}

func TestDecideExistingInstanceActionSkipsBrowserReopenWhileOtherInstanceStarts(t *testing.T) {
	action := decideExistingInstanceAction(Config{Mode: ModeBrowser}, &lockState{pid: os.Getpid(), port: 0, reused: true})

	if action.continueStartup {
		t.Fatal("continueStartup = true, want false")
	}
	if action.openBrowser {
		t.Fatal("openBrowser = true, want false")
	}
}

func TestDecideExistingInstanceActionExitsQuietlyForWebviewInstance(t *testing.T) {
	action := decideExistingInstanceAction(Config{Mode: ModeWebview}, &lockState{pid: os.Getpid(), port: 43123, reused: true})

	if action.continueStartup {
		t.Fatal("continueStartup = true, want false")
	}
	if action.openBrowser {
		t.Fatal("openBrowser = true, want false")
	}
	if action.browserURL != "" {
		t.Fatalf("browserURL = %q, want empty", action.browserURL)
	}
}

func TestDecideExistingInstanceActionExitsQuietlyForHeadlessRelaunch(t *testing.T) {
	action := decideExistingInstanceAction(Config{Mode: ModeBrowser, Headless: true}, &lockState{pid: os.Getpid(), port: 43123, reused: true})

	if action.continueStartup {
		t.Fatal("continueStartup = true, want false")
	}
	if action.openBrowser {
		t.Fatal("openBrowser = true, want false")
	}
}

func TestRunWebviewStubReportsRebuildGuidance(t *testing.T) {
	err := runWebview(&Runtime{})
	if err == nil {
		t.Fatal("runWebview() error = nil, want rebuild guidance")
	}
	const want = "webview mode is not available in this build; rebuild with -tags webview"
	if err.Error() != want {
		t.Fatalf("runWebview() error = %q, want %q", err.Error(), want)
	}
}
