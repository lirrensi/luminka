//go:build webview

package main

import (
	"testing"

	"luminka/luminka"
)

func TestAppModeWebviewBuild(t *testing.T) {
	if got := appMode(); got != luminka.ModeWebview {
		t.Fatalf("appMode() = %s, want %s", got, luminka.ModeWebview)
	}
}
