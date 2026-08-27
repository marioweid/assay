package httpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	t.Parallel()

	readyCalls := 0
	handler := NewHandler(readinessFunc(func(context.Context) error {
		readyCalls++
		return nil
	}), discardLogger())

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q", response.Header().Get("Content-Type"))
	}
	if response.Body.String() != "ok\n" {
		t.Errorf("body = %q, want %q", response.Body.String(), "ok\n")
	}
	if readyCalls != 0 {
		t.Errorf("readiness calls = %d, want 0", readyCalls)
	}
}

func TestReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		readyError error
		wantStatus int
		wantBody   string
	}{
		{name: "ready", wantStatus: http.StatusOK, wantBody: "ready\n"},
		{
			name:       "database unavailable",
			readyError: errors.New("database unavailable"),
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "not ready\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler := NewHandler(readinessFunc(func(context.Context) error {
				return test.readyError
			}), discardLogger())
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

			if response.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if response.Body.String() != test.wantBody {
				t.Errorf("body = %q, want %q", response.Body.String(), test.wantBody)
			}
		})
	}
}

func TestHealthEndpointsRejectOtherMethods(t *testing.T) {
	t.Parallel()

	handler := NewHandler(readinessFunc(func(context.Context) error { return nil }), discardLogger())
	for _, path := range []string{"/healthz", "/readyz"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s status = %d, want %d", path, response.Code, http.StatusMethodNotAllowed)
		}
	}
}

type readinessFunc func(context.Context) error

func (f readinessFunc) Ready(ctx context.Context) error {
	return f(ctx)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
