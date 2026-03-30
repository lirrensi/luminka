//go:build !webview

// FILE: luminka/webview_other.go
// PURPOSE: Report that webview mode is unavailable in non-webview builds.
// OWNS: The Phase 1 unsupported webview-mode error path.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md

package luminka

import "fmt"

func runWebview(rt *Runtime) error {
	if rt == nil {
		return fmt.Errorf("webview mode is not available in this build; rebuild with -tags webview")
	}
	return fmt.Errorf("webview mode is not available in this build; rebuild with -tags webview")
}
