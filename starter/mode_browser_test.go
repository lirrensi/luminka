//go:build !webview

package main

import (
	"testing"

	"github.com/lirrensi/luminka/luminka"
)

func TestAppModeBrowserBuild(t *testing.T) {
	if got := appMode(); got != luminka.ModeBrowser {
		t.Fatalf("appMode() = %s, want %s", got, luminka.ModeBrowser)
	}
}
