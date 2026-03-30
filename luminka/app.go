// FILE: luminka/app.go
// PURPOSE: Orchestrate Luminka runtime startup, capability resolution, and shutdown flow.
// OWNS: Config and Mode definitions, runtime state, capability state, and startup/shutdown flow.
// EXPORTS: Mode, ModeBrowser, ModeWebview, Config, Runtime, Run
// DOCS: docs/spec.md, docs/arch.md

package luminka

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

const defaultAppName = "luminka"

type Mode string

const (
	ModeBrowser Mode = "browser"
	ModeWebview Mode = "webview"
)

type Config struct {
	Name            string
	Mode            Mode
	Port            int
	Idle            time.Duration
	WindowTitle     string
	WindowWidth     int
	WindowHeight    int
	WindowResizable bool
	WindowDebug     bool
	DisableFS       bool
	EnableScripts   bool
	EnableShell     bool
	ExecTimeout     time.Duration
	Root            string
	Assets          fs.FS
	ScriptAssets    fs.FS
}

type capabilityState struct {
	FS      bool `json:"fs"`
	Scripts bool `json:"scripts"`
	Shell   bool `json:"shell"`
}

type Runtime struct {
	Name            string
	Mode            Mode
	Root            string
	Idle            time.Duration
	ExecTimeout     time.Duration
	WindowTitle     string
	WindowWidth     int
	WindowHeight    int
	WindowResizable bool
	WindowDebug     bool
	Capabilities    capabilityState
	FSBridge        *FSBridge
	Watcher         *Watcher
	ScriptBridge    *ScriptBridge
	ShellBridge     *ShellBridge

	Assets       fs.FS
	ScriptAssets fs.FS
	Port         int

	LockPath  string
	PID       int
	ownedLock bool

	connections map[*wsConnection]struct{}
	mu          sync.Mutex
	idleTimer   *time.Timer

	shutdownCh   chan struct{}
	shutdownOnce sync.Once
	listener     net.Listener
	server       *http.Server
}

type lockState struct {
	path   string
	pid    int
	port   int
	owned  bool
	reused bool
}

type existingInstanceAction struct {
	continueStartup bool
	openBrowser     bool
	browserURL      string
}

func Run(cfg Config) (err error) {
	cfg = normalizeConfig(cfg)
	if cfg.Assets == nil {
		return errors.New("assets are required")
	}

	rt, existing, err := prepareRuntime(cfg)
	if err != nil {
		return err
	}
	if action := decideExistingInstanceAction(cfg, existing); !action.continueStartup {
		if action.openBrowser {
			return openBrowser(action.browserURL)
		}
		return nil
	}

	defer func() {
		if cleanupErr := rt.cleanup(); err == nil && cleanupErr != nil {
			err = cleanupErr
		}
	}()

	if err = startServer(rt); err != nil {
		return err
	}
	if err = writeLockPort(rt.LockPath, rt.PID, rt.Port); err != nil {
		return err
	}

	switch rt.Mode {
	case ModeBrowser:
		err = runBrowser(rt)
	case ModeWebview:
		err = runWebview(rt)
	default:
		err = fmt.Errorf("unsupported mode %q", rt.Mode)
	}
	return err
}

func decideExistingInstanceAction(cfg Config, existing *lockState) existingInstanceAction {
	if existing == nil {
		return existingInstanceAction{continueStartup: true}
	}
	switch cfg.Mode {
	case ModeBrowser:
		if existing.port > 0 {
			return existingInstanceAction{openBrowser: true, browserURL: localURL(existing.port)}
		}
		return existingInstanceAction{}
	case ModeWebview:
		return existingInstanceAction{}
	default:
		return existingInstanceAction{}
	}
}

func normalizeConfig(cfg Config) Config {
	if cfg.Name == "" {
		cfg.Name = defaultAppName
	}
	if cfg.Mode == "" {
		cfg.Mode = ModeBrowser
	}
	if cfg.WindowTitle == "" {
		cfg.WindowTitle = cfg.Name
	}
	if cfg.WindowWidth == 0 {
		cfg.WindowWidth = 1280
	}
	if cfg.WindowHeight == 0 {
		cfg.WindowHeight = 800
	}
	if cfg.Idle == 0 {
		cfg.Idle = defaultIdleTimeout
	}
	if cfg.ExecTimeout == 0 {
		cfg.ExecTimeout = 30 * time.Second
	}
	return cfg
}

func prepareRuntime(cfg Config) (*Runtime, *lockState, error) {
	root, err := resolveRootDirectory(cfg.Root)
	if err != nil {
		return nil, nil, err
	}

	state, err := acquireInstanceLock(root, cfg.Name)
	if err != nil {
		return nil, nil, err
	}
	if state.reused {
		return nil, state, nil
	}

	rt := &Runtime{
		Name:            cfg.Name,
		Mode:            cfg.Mode,
		Root:            root,
		Idle:            cfg.Idle,
		ExecTimeout:     cfg.ExecTimeout,
		WindowTitle:     cfg.WindowTitle,
		WindowWidth:     cfg.WindowWidth,
		WindowHeight:    cfg.WindowHeight,
		WindowResizable: cfg.WindowResizable,
		WindowDebug:     cfg.WindowDebug,
		Assets:          cfg.Assets,
		ScriptAssets:    cfg.ScriptAssets,
		LockPath:        state.path,
		PID:             state.pid,
		ownedLock:       state.owned,
		connections:     make(map[*wsConnection]struct{}),
		shutdownCh:      make(chan struct{}),
	}

	rt.FSBridge = NewFSBridge(root)
	rt.Capabilities = capabilityState{
		FS:      !cfg.DisableFS,
		Scripts: cfg.EnableScripts && scriptSupportAvailable(),
		Shell:   cfg.EnableShell && shellSupportAvailable(),
	}
	rt.Watcher = NewWatcher(root, time.Second, func(path string) error {
		return rt.pushFSChanged(path)
	})
	rt.ScriptBridge = NewScriptBridge(root, cfg.ExecTimeout)
	rt.ScriptBridge.scriptAssets = cfg.ScriptAssets
	rt.ShellBridge = NewShellBridge(root, cfg.ExecTimeout)

	return rt, nil, nil
}

func (rt *Runtime) requestShutdown() {
	if rt == nil {
		return
	}
	rt.shutdownOnce.Do(func() {
		close(rt.shutdownCh)
	})
}

func (rt *Runtime) cleanup() error {
	if rt == nil {
		return nil
	}

	rt.stopIdleTimer()
	if rt.Watcher != nil {
		rt.Watcher.Stop()
	}

	var firstErr error
	if rt.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := rt.server.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if rt.listener != nil {
		if err := rt.listener.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if rt.ownedLock {
		if err := removeLockFile(rt.LockPath); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
