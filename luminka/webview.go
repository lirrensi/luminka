//go:build webview

// FILE: luminka/webview.go
// PURPOSE: Drive the native Luminka webview window lifecycle in webview builds.
// OWNS: Webview window creation, navigation, shutdown, and destruction.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md

package luminka

import (
	"errors"

	"github.com/webview/webview_go"
)

func runWebview(rt *Runtime) error {
	if rt == nil {
		return errors.New("runtime is required")
	}

	wv := webview.New(rt.WindowDebug)
	if wv == nil {
		return errors.New("failed to create webview instance")
	}
	defer wv.Destroy()

	wv.SetTitle(rt.WindowTitle)
	var hint webview.Hint = webview.HintFixed
	if rt.WindowResizable {
		hint = webview.HintNone
	}
	wv.SetSize(rt.WindowWidth, rt.WindowHeight, hint)
	wv.Navigate(localURL(rt.Port))

	go func() {
		<-rt.shutdownCh
		wv.Terminate()
	}()

	wv.Run()
	return nil
}
