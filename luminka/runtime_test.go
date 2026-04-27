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
	if got.AppVersion != defaultAppVersion {
		t.Fatalf("AppVersion = %q, want %q", got.AppVersion, defaultAppVersion)
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
	if rt.AppVersion != defaultAppVersion {
		t.Fatalf("AppVersion = %q, want %q", rt.AppVersion, defaultAppVersion)
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
	if state.record.PID != os.Getpid() {
		t.Fatalf("pid = %d, want %d", state.record.PID, os.Getpid())
	}
	if state.record.Port != 0 {
		t.Fatalf("port = %d, want 0", state.record.Port)
	}

	record, err := readLockRecord(state.path)
	if err != nil {
		t.Fatalf("readLockRecord() error = %v", err)
	}
	if record.PID != os.Getpid() || record.Port != 0 {
		t.Fatalf("lock record = %#v, want current pid and port 0", record)
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
	if second.record.PID != os.Getpid() {
		t.Fatalf("pid = %d, want %d", second.record.PID, os.Getpid())
	}
	if second.record.Port != 0 {
		t.Fatalf("port = %d, want 0", second.record.Port)
	}

	record, err := readLockRecord(first.path)
	if err != nil {
		t.Fatalf("readLockRecord() error = %v", err)
	}
	if record.PID != os.Getpid() || record.Port != 0 {
		t.Fatalf("lock record = %#v, want current pid and port 0", record)
	}
}

func TestAcquireInstanceLockRecoversStalePIDZeroRecord(t *testing.T) {
	const stalePID = 999999
	if processAlive(stalePID) {
		t.Skipf("pid %d is unexpectedly alive on this system", stalePID)
	}

	root := t.TempDir()
	path := lockFilePath(root, "runtime-lock-stale")
	if err := writeInstanceRecord(path, instanceRecord{PID: stalePID, Port: 0, Mode: ModeBrowser}); err != nil {
		t.Fatalf("writeInstanceRecord() error = %v", err)
	}

	state, err := acquireInstanceLock(root, "runtime-lock-stale")
	if err != nil {
		t.Fatalf("acquireInstanceLock() error = %v", err)
	}
	t.Cleanup(func() { _ = removeLockFile(state.path) })

	if !state.owned || state.reused {
		t.Fatalf("lock state = %#v, want fresh owned lock after stale recovery", state)
	}
	if state.record.PID != os.Getpid() {
		t.Fatalf("pid = %d, want %d", state.record.PID, os.Getpid())
	}

	record, err := readLockRecord(path)
	if err != nil {
		t.Fatalf("readLockRecord() error = %v", err)
	}
	if record.PID != os.Getpid() || record.Port != 0 {
		t.Fatalf("lock record = %#v, want current pid and port 0", record)
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
	if err := writeInstanceRecord(path, instanceRecord{PID: os.Getpid(), Port: port, Mode: ModeBrowser}); err != nil {
		t.Fatalf("writeInstanceRecord() error = %v", err)
	}

	state, err := acquireInstanceLock(root, "runtime-lock-live-port")
	if err != nil {
		t.Fatalf("acquireInstanceLock() error = %v", err)
	}

	if state == nil || !state.reused || state.owned {
		t.Fatalf("lock state = %#v, want reused reachable-port lock", state)
	}
	if state.record.PID != os.Getpid() {
		t.Fatalf("pid = %d, want %d", state.record.PID, os.Getpid())
	}
	if state.record.Port != port {
		t.Fatalf("port = %d, want %d", state.record.Port, port)
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
	if err := writeInstanceRecord(path, instanceRecord{PID: os.Getpid(), Port: port, Mode: ModeBrowser}); err != nil {
		t.Fatalf("writeInstanceRecord() error = %v", err)
	}

	state, err := acquireInstanceLock(root, "runtime-lock-closed-port")
	if err != nil {
		t.Fatalf("acquireInstanceLock() error = %v", err)
	}
	t.Cleanup(func() { _ = removeLockFile(state.path) })

	if !state.owned || state.reused {
		t.Fatalf("lock state = %#v, want fresh owned lock after closed-port recovery", state)
	}
	if state.record.PID != os.Getpid() {
		t.Fatalf("pid = %d, want %d", state.record.PID, os.Getpid())
	}
	if state.record.Port != 0 {
		t.Fatalf("port = %d, want 0", state.record.Port)
	}

	record, err := readLockRecord(path)
	if err != nil {
		t.Fatalf("readLockRecord() error = %v", err)
	}
	if record.PID != os.Getpid() || record.Port != 0 {
		t.Fatalf("lock record = %#v, want current pid and port 0", record)
	}
}

func TestWriteInstanceRecordPreservesModeAndWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.lock")
	want := instanceRecord{
		PID:  os.Getpid(),
		Port: 43210,
		Mode: ModeWebview,
		Window: instanceWindowRecord{
			Platform: "windows",
			ID:       "0x1234",
		},
	}
	if err := writeInstanceRecord(path, want); err != nil {
		t.Fatalf("writeInstanceRecord() error = %v", err)
	}
	got, err := readLockRecord(path)
	if err != nil {
		t.Fatalf("readLockRecord() error = %v", err)
	}
	if *got != want {
		t.Fatalf("record = %#v, want %#v", *got, want)
	}
}

func TestReadLockRecordAcceptsLegacyTextRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.lock")
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d:%d", os.Getpid(), 12345)), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	record, err := readLockRecord(path)
	if err != nil {
		t.Fatalf("readLockRecord() error = %v", err)
	}
	if record.PID != os.Getpid() || record.Port != 12345 {
		t.Fatalf("record = %#v, want legacy pid and port", record)
	}
}

func TestDecideExistingInstanceActionOpensBrowserForRunningBrowserInstance(t *testing.T) {
	action := decideExistingInstanceAction(Config{Mode: ModeBrowser}, &lockState{record: instanceRecord{PID: os.Getpid(), Port: 43123}, reused: true})

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
	action := decideExistingInstanceAction(Config{Mode: ModeBrowser}, &lockState{record: instanceRecord{PID: os.Getpid(), Port: 0}, reused: true})

	if action.continueStartup {
		t.Fatal("continueStartup = true, want false")
	}
	if action.openBrowser {
		t.Fatal("openBrowser = true, want false")
	}
}

func TestDecideExistingInstanceActionExitsQuietlyForWebviewInstance(t *testing.T) {
	record := instanceRecord{PID: os.Getpid(), Port: 43123, Mode: ModeWebview}
	action := decideExistingInstanceAction(Config{Mode: ModeWebview}, &lockState{record: record, reused: true})

	if action.continueStartup {
		t.Fatal("continueStartup = true, want false")
	}
	if action.openBrowser {
		t.Fatal("openBrowser = true, want false")
	}
	if action.browserURL != "" {
		t.Fatalf("browserURL = %q, want empty", action.browserURL)
	}
	if !action.focusExisting {
		t.Fatal("focusExisting = false, want true")
	}
	if action.record != record {
		t.Fatalf("record = %#v, want %#v", action.record, record)
	}
}

func TestDecideExistingInstanceActionExitsQuietlyForHeadlessRelaunch(t *testing.T) {
	action := decideExistingInstanceAction(Config{Mode: ModeBrowser, Headless: true}, &lockState{record: instanceRecord{PID: os.Getpid(), Port: 43123}, reused: true})

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
