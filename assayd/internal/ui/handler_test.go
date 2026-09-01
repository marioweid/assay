package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

type handlerCase struct {
	name        string
	method      string
	path        string
	status      int
	body        string
	contentType string
	cache       string
}

func TestHandlerServesAssetsAndSPARoutes(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{
		"index.html":           &fstest.MapFile{Data: []byte("<!doctype html><div id=app></div>")},
		"assay-icon.png":       &fstest.MapFile{Data: []byte("icon")},
		"assets/app-a1b2.js":   &fstest.MapFile{Data: []byte("console.log('assay')")},
		"assets/font-a1.woff2": &fstest.MapFile{Data: []byte("font")},
	}
	handler := newHandler(files, true)
	tests := []handlerCase{
		{name: "index", method: http.MethodGet, path: "/", status: http.StatusOK,
			body: "id=app", contentType: "text/html; charset=utf-8", cache: "no-cache"},
		{name: "deep link", method: http.MethodGet, path: "/apps/one/traces/two",
			status: http.StatusOK, body: "id=app", contentType: "text/html; charset=utf-8",
			cache: "no-cache"},
		{name: "javascript", method: http.MethodGet, path: "/assets/app-a1b2.js",
			status: http.StatusOK, body: "console.log", contentType: "text/javascript; charset=utf-8",
			cache: "public, max-age=31536000, immutable"},
		{name: "font", method: http.MethodGet, path: "/assets/font-a1.woff2",
			status: http.StatusOK, body: "font", contentType: "font/woff2",
			cache: "public, max-age=31536000, immutable"},
		{name: "public image", method: http.MethodGet, path: "/assay-icon.png",
			status: http.StatusOK, body: "icon", cache: "no-cache"},
		{name: "missing extension", method: http.MethodGet, path: "/assets/missing.js",
			status: http.StatusNotFound},
		{name: "post", method: http.MethodPost, path: "/apps", status: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertHandlerCase(t, handler, test)
		})
	}
}

func assertHandlerCase(t *testing.T, handler http.Handler, test handlerCase) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
	if response.Code != test.status {
		t.Fatalf("status = %d, want %d", response.Code, test.status)
	}
	if test.body != "" && !strings.Contains(response.Body.String(), test.body) {
		t.Fatalf("body = %q, want substring %q", response.Body.String(), test.body)
	}
	if test.contentType != "" && response.Header().Get("Content-Type") != test.contentType {
		t.Fatalf("content type = %q, want %q", response.Header().Get("Content-Type"), test.contentType)
	}
	if test.cache != "" && response.Header().Get("Cache-Control") != test.cache {
		t.Fatalf("cache = %q, want %q", response.Header().Get("Cache-Control"), test.cache)
	}
	assertSecurityHeaders(t, response.Header())
}

func TestHandlerHEADAndDisabled(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("index")}}
	t.Run("head omits body", func(t *testing.T) {
		response := httptest.NewRecorder()
		newHandler(files, true).ServeHTTP(response, httptest.NewRequest(http.MethodHead, "/route", nil))
		if response.Code != http.StatusOK || response.Body.Len() != 0 {
			t.Fatalf("HEAD status/body = %d/%q", response.Code, response.Body.String())
		}
	})
	t.Run("disabled", func(t *testing.T) {
		response := httptest.NewRecorder()
		newHandler(files, false).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("disabled status = %d, want 404", response.Code)
		}
	})
}

func TestRegisterServesEmbeddedPlaceholder(t *testing.T) {
	t.Parallel()
	router := http.NewServeMux()
	Register(router, true)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/apps/example/traces", nil))
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), "UI assets were not built") {
		t.Fatalf("embedded route status/body = %d/%q", response.Code, response.Body.String())
	}
}

func assertSecurityHeaders(t *testing.T, header http.Header) {
	t.Helper()
	wants := map[string]string{
		"Content-Security-Policy": contentSecurityPolicy,
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
	}
	for name, want := range wants {
		if got := header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}
