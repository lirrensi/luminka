// FILE: examples/hello/main.go
// PURPOSE: Start the smallest Luminka example with filesystem capability disabled.
// OWNS: Hello example entrypoint wiring, embedded assets, and runtime config.
// EXPORTS: main
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_luminka_phase3_surface_examples_2026-03-30.md

package main

import (
	"embed"
	"log"

	"github.com/lirrensi/luminka/luminka"
)

//go:embed dist/*
var distAssets embed.FS

func main() {
	if err := luminka.Run(luminka.Config{
		Name:            "luminka-hello",
		Mode:            appMode(),
		RootPolicy:      luminka.RootPolicyPortable,
		WindowTitle:     "luminka-hello",
		WindowWidth:     1280,
		WindowHeight:    800,
		WindowResizable: true,
		WindowDebug:     false,
		DisableFS:       true,
		EnableScripts:   false,
		EnableShell:     false,
		Assets:          distAssets,
	}); err != nil {
		log.Fatal(err)
	}
}
