// FILE: luminka/server_test.go
// PURPOSE: Verify embedded asset serving behavior, including SPA entry fallback for unknown routes.
// OWNS: Deterministic handler tests for buildAssetHandler.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_spa_fallback_2026-03-30.md

package luminka

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBuildAssetHandlerServesRootAndIndexDocument(t *testing.T) {
	handler, err := buildAssetHandler(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>root</html>")},
	})
	if err != nil {
		t.Fatalf("buildAssetHandler() error = %v", err)
	}

	for _, target := range []string{"/", "/index.html"} {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}
			if body := rr.Body.String(); body != "<html>root</html>" {
				t.Fatalf("body = %q, want root document", body)
			}
		})
	}
}

func TestBuildAssetHandlerFallsBackToDistAssets(t *testing.T) {
	handler, err := buildAssetHandler(fstest.MapFS{
		"dist/index.html": &fstest.MapFile{Data: []byte("<html>dist index</html>")},
		"dist/app.js":     &fstest.MapFile{Data: []byte("console.log('dist')")},
	})
	if err != nil {
		t.Fatalf("buildAssetHandler() error = %v", err)
	}

	t.Run("root falls back to dist index", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		if body := rr.Body.String(); body != "<html>dist index</html>" {
			t.Fatalf("body = %q, want dist index document", body)
		}
	})

	t.Run("asset path falls back to dist file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		if body := rr.Body.String(); body != "console.log('dist')" {
			t.Fatalf("body = %q, want dist asset", body)
		}
		if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
			t.Fatalf("Content-Type = %q, want javascript content type", got)
		}
	})
}

func TestBuildAssetHandlerHeadReturnsHeadersWithoutBody(t *testing.T) {
	handler, err := buildAssetHandler(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>root</html>")},
	})
	if err != nil {
		t.Fatalf("buildAssetHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if body := rr.Body.String(); body != "" {
		t.Fatalf("body = %q, want empty for HEAD", body)
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("Content-Type = %q, want html content type", got)
	}
}

func TestBuildAssetHandlerServesStaticSubtreeWhenPresent(t *testing.T) {
	handler, err := buildAssetHandler(fstest.MapFS{
		"static/index.html": &fstest.MapFile{Data: []byte("<html>static root</html>")},
		"static/app.js":     &fstest.MapFile{Data: []byte("console.log('static')")},
	})
	if err != nil {
		t.Fatalf("buildAssetHandler() error = %v", err)
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	resp, err := http.Get(server.URL + "/app.js")
	if err != nil {
		t.Fatalf("http.Get() error = %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := string(body); got != "console.log('static')" {
		t.Fatalf("body = %q, want static asset", got)
	}
}

func TestBuildAssetHandlerFallsBackToSPAEntryForUnknownDeepLink(t *testing.T) {
	handler, err := buildAssetHandler(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>spa entry</html>")},
	})
	if err != nil {
		t.Fatalf("buildAssetHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/boards/123", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if body := rr.Body.String(); body != "<html>spa entry</html>" {
		t.Fatalf("body = %q, want spa entry document", body)
	}
}

func TestBuildAssetHandlerFallsBackToSPAEntryForUnknownHeadDeepLink(t *testing.T) {
	handler, err := buildAssetHandler(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>spa entry</html>")},
	})
	if err != nil {
		t.Fatalf("buildAssetHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodHead, "/settings/profile", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("Content-Type = %q, want html content type", got)
	}
	if body := rr.Body.String(); body != "" {
		t.Fatalf("body = %q, want empty for HEAD", body)
	}
}

func TestBuildAssetHandlerPrefersRealAssetOverSPAFallback(t *testing.T) {
	handler, err := buildAssetHandler(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>spa entry</html>")},
		"app.js":     &fstest.MapFile{Data: []byte("console.log('real asset')")},
	})
	if err != nil {
		t.Fatalf("buildAssetHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if body := rr.Body.String(); body != "console.log('real asset')" {
		t.Fatalf("body = %q, want real asset", body)
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("Content-Type = %q, want javascript content type", got)
	}
}

func TestBuildAssetHandlerReturnsNotFoundWithoutEntryDocument(t *testing.T) {
	handler, err := buildAssetHandler(fstest.MapFS{
		"app.js": &fstest.MapFile{Data: []byte("console.log('real asset')")},
	})
	if err != nil {
		t.Fatalf("buildAssetHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/boards/123", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}
