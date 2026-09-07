package ui

import (
	"bytes"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

const (
	immutableCache        = "public, max-age=31536000, immutable"
	contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; " +
		"img-src 'self'; font-src 'self'; connect-src 'self'; object-src 'none'; " +
		"frame-ancestors 'none'"
)

type assetHandler struct {
	files   fs.FS
	enabled bool
}

// Register mounts the embedded browser interface at the root of router.
func Register(router *http.ServeMux, enabled bool) {
	files, err := fs.Sub(embeddedAssets, "dist")
	if err != nil {
		files = nil
	}
	router.Handle("/", newHandler(files, enabled))
}

func newHandler(files fs.FS, enabled bool) http.Handler {
	return &assetHandler{files: files, enabled: enabled}
}

func (h *assetHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setSecurityHeaders(writer.Header())
	if !h.enabled || (request.Method != http.MethodGet && request.Method != http.MethodHead) {
		http.NotFound(writer, request)
		return
	}
	if h.files == nil {
		http.Error(writer, "UI assets are unavailable", http.StatusInternalServerError)
		return
	}
	if hasParentSegment(request.URL.Path) {
		http.NotFound(writer, request)
		return
	}
	h.servePath(writer, request)
}

func (h *assetHandler) servePath(writer http.ResponseWriter, request *http.Request) {
	requested := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
	if requested == "" {
		requested = "index.html"
	}
	data, err := fs.ReadFile(h.files, requested)
	if err == nil {
		h.serveAsset(writer, request, requested, data)
		return
	}
	if path.Ext(path.Base(requested)) != "" {
		http.NotFound(writer, request)
		return
	}
	data, err = fs.ReadFile(h.files, "index.html")
	if err != nil {
		http.Error(writer, "UI index is unavailable", http.StatusInternalServerError)
		return
	}
	h.serveAsset(writer, request, "index.html", data)
}

func (h *assetHandler) serveAsset(
	writer http.ResponseWriter,
	request *http.Request,
	name string,
	data []byte,
) {
	if strings.HasPrefix(name, "assets/") {
		writer.Header().Set("Cache-Control", immutableCache)
	} else {
		writer.Header().Set("Cache-Control", "no-cache")
	}
	contentType := assetContentType(name)
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	writer.Header().Set("Content-Type", contentType)
	http.ServeContent(writer, request, name, time.Time{}, bytes.NewReader(data))
}

func assetContentType(name string) string {
	switch path.Ext(name) {
	case ".css":
		return "text/css; charset=utf-8"
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	default:
		return mime.TypeByExtension(path.Ext(name))
	}
}

func setSecurityHeaders(header http.Header) {
	header.Set("Content-Security-Policy", contentSecurityPolicy)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("Referrer-Policy", "no-referrer")
}

func hasParentSegment(requestPath string) bool {
	for _, segment := range strings.Split(requestPath, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}
