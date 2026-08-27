// Package httpserver serves Assay's HTTP endpoints and owns server lifecycle.
package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
)

// Readiness checks whether the application can serve traffic.
type Readiness interface {
	Ready(context.Context) error
}

// NewHandler returns Assay's M0 HTTP routes.
func NewHandler(ready Readiness, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writePlain(writer, http.StatusOK, "ok\n", logger)
	})
	mux.HandleFunc("GET /readyz", func(writer http.ResponseWriter, request *http.Request) {
		if err := ready.Ready(request.Context()); err != nil {
			logger.Debug("database readiness check failed", "error", err)
			writePlain(writer, http.StatusServiceUnavailable, "not ready\n", logger)
			return
		}
		writePlain(writer, http.StatusOK, "ready\n", logger)
	})
	return mux
}

func writePlain(writer http.ResponseWriter, status int, body string, logger *slog.Logger) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(status)
	if _, err := io.WriteString(writer, body); err != nil {
		logger.Debug("write health response", "error", err)
	}
}
