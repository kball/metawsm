package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRegisterSPA(t *testing.T) {
	publicFS := fstest.MapFS{
		"index.html":     {Data: []byte("<html><body>index</body></html>")},
		"assets/app.js":  {Data: []byte("console.log('ok');")},
		"assets/app.css": {Data: []byte("body{background:#fff;}")},
	}

	mux := http.NewServeMux()
	mounted := RegisterSPA(mux, publicFS, SPAOptions{APIPrefix: "/api", WSPath: "/ws"})
	if !mounted {
		t.Fatalf("expected SPA handler to mount")
	}

	check := func(path string, wantStatus int, wantBodyContains string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != wantStatus {
			t.Fatalf("path %s: expected status %d, got %d", path, wantStatus, rr.Code)
		}
		if wantBodyContains != "" && !strings.Contains(rr.Body.String(), wantBodyContains) {
			t.Fatalf("path %s: expected body to contain %q, got %q", path, wantBodyContains, rr.Body.String())
		}
	}

	check("/", http.StatusOK, "index")
	check("/assets/app.js", http.StatusOK, "console.log")
	check("/runs/some-route", http.StatusOK, "index")
	check("/api/v1/runs", http.StatusNotFound, "404")
}

func TestRegisterSPAMissingIndex(t *testing.T) {
	publicFS := fstest.MapFS{
		"assets/app.js": {Data: []byte("console.log('ok');")},
	}

	mux := http.NewServeMux()
	mounted := RegisterSPA(mux, fs.FS(publicFS), SPAOptions{})
	if mounted {
		t.Fatalf("expected SPA handler not to mount without index.html")
	}
}
