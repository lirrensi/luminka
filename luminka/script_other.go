//go:build !scripts

// FILE: luminka/script_other.go
// PURPOSE: Report that the constrained script lane is unavailable in non-scripts builds.
// OWNS: Script bridge stubbing and capability availability reporting.
// EXPORTS: ScriptBridge, NewScriptBridge, scriptSupportAvailable
// DOCS: docs/spec.md, docs/arch.md

package luminka

import (
	"errors"
	"io/fs"
	"time"
)

type ScriptBridge struct {
	root           string
	defaultTimeout time.Duration
	scriptAssets   fs.FS
}

func NewScriptBridge(root string, defaultTimeout time.Duration) *ScriptBridge {
	return &ScriptBridge{root: root, defaultTimeout: defaultTimeout}
}

func scriptSupportAvailable() bool {
	return false
}

func (sb *ScriptBridge) Exec(runner string, file string, args []string, timeout time.Duration) (stdout string, stderr string, code int, err error) {
	return "", "", -1, errors.New("script support is not available in this build; rebuild with -tags scripts")
}
