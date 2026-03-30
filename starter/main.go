// FILE: starter/main.go
// PURPOSE: Start the canonical Luminka starter app from embedded dist assets.
// OWNS: Starter entrypoint wiring, embedded assets, and app-level runtime config.
// EXPORTS: main
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_luminka_phase3_surface_examples_2026-03-30.md

package main

import (
	"embed"
	"log"

	"luminka/luminka"
)

//go:embed dist/*
var distAssets embed.FS

func main() {
	if err := luminka.Run(luminka.Config{
		Name:            "luminka-starter",
		Mode:            appMode(),
		WindowTitle:     "luminka-starter",
		WindowWidth:     1280,
		WindowHeight:    800,
		WindowResizable: true,
		WindowDebug:     false,
		EnableScripts:   false,
		EnableShell:     false,
		Assets:          distAssets,
	}); err != nil {
		log.Fatal(err)
	}
}
