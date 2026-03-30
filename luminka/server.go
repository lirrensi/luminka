// FILE: luminka/server.go
// PURPOSE: Start the loopback HTTP server and serve embedded frontend assets.
// OWNS: Port binding, mux creation, /ws registration, and static asset serving.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_phase1_runtime_2026-03-30.md, agent_chat/plan_luminka_spa_fallback_2026-03-30.md

package luminka

import (
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"path"
	"strings"
)

func startServer(rt *Runtime) error {
	if rt == nil {
		return errors.New("runtime is required")
	}
	if rt.Assets == nil {
		return errors.New("embedded assets are required")
	}

	assetHandler, err := buildAssetHandler(rt.Assets)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", rt.serveWebSocket)
	mux.Handle("/", assetHandler)

	addr := "127.0.0.1:0"
	if rt.Port != 0 {
		addr = fmt.Sprintf("127.0.0.1:%d", rt.Port)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	rt.listener = ln
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		rt.Port = tcpAddr.Port
	}
	rt.server = &http.Server{Handler: mux}

	go func() {
		_ = rt.server.Serve(ln)
	}()

	return nil
}

func buildAssetHandler(assets fs.FS) (http.Handler, error) {
	if assets == nil {
		return nil, errors.New("embedded assets are required")
	}

	if info, err := fs.Stat(assets, "static"); err == nil && info.IsDir() {
		if sub, err := fs.Sub(assets, "static"); err == nil {
			assets = sub
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		for _, candidate := range assetCandidates(name) {
			if serveEmbeddedFile(w, r, assets, candidate) {
				return
			}
		}
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			for _, candidate := range fallbackEntryCandidates() {
				if serveEmbeddedFile(w, r, assets, candidate) {
					return
				}
			}
		}
		http.NotFound(w, r)
	}), nil
}

func assetCandidates(name string) []string {
	if name == "" {
		name = "index.html"
	}
	return []string{name, path.Join("dist", name), path.Join("static", name)}
}

func fallbackEntryCandidates() []string {
	return []string{"index.html", path.Join("dist", "index.html"), path.Join("static", "index.html")}
}

func serveEmbeddedFile(w http.ResponseWriter, r *http.Request, assets fs.FS, name string) bool {
	data, err := fs.ReadFile(assets, name)
	if err != nil {
		return false
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
	return true
}
