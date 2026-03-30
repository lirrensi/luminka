// FILE: luminka/browser.go
// PURPOSE: Launch the browser shell and manage the browser-mode idle lifecycle.
// OWNS: Browser launch, idle timer behavior, and browser-mode shutdown waiting.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_phase1_runtime_2026-03-30.md

package luminka

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

const defaultIdleTimeout = 180 * time.Second

func runBrowser(rt *Runtime) error {
	if err := openBrowser(localURL(rt.Port)); err != nil {
		return err
	}
	rt.startIdleTimer()
	return rt.waitForShutdown()
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("unsupported operating system %q", runtime.GOOS)
	}
	return cmd.Start()
}

func localURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func (rt *Runtime) waitForShutdown() error {
	if rt == nil {
		return nil
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case <-rt.shutdownCh:
		return nil
	case <-sigCh:
		rt.requestShutdown()
		<-rt.shutdownCh
		return nil
	}
}

func (rt *Runtime) startIdleTimer() {
	if rt == nil {
		return
	}

	idle := rt.Idle
	if idle == 0 {
		idle = defaultIdleTimeout
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.idleTimer != nil {
		rt.idleTimer.Stop()
		rt.idleTimer = nil
	}
	if len(rt.connections) != 0 {
		return
	}

	rt.idleTimer = time.AfterFunc(idle, func() {
		if rt.connectionCount() == 0 {
			rt.requestShutdown()
		}
	})
}

func (rt *Runtime) stopIdleTimer() {
	if rt == nil {
		return
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.idleTimer != nil {
		rt.idleTimer.Stop()
		rt.idleTimer = nil
	}
}

func (rt *Runtime) connectionCount() int {
	if rt == nil {
		return 0
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	return len(rt.connections)
}
