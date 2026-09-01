package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/marioweid/assay/assayd/internal/config"
)

func TestRegisterRoutesKeepsServiceRoutePrecedence(t *testing.T) {
	t.Parallel()
	router := http.NewServeMux()
	router.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	application := &App{
		config: config.Config{AdminToken: "admin-secret", UIEnabled: true},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	application.registerRoutes(router)

	tests := []struct {
		method string
		path   string
		status int
	}{
		{method: http.MethodGet, path: "/healthz", status: http.StatusNoContent},
		{method: http.MethodGet, path: "/openapi.json", status: http.StatusOK},
		{method: http.MethodGet, path: "/docs", status: http.StatusOK},
		{method: http.MethodGet, path: "/v1/projects", status: http.StatusUnauthorized},
		{method: http.MethodPost, path: "/v1/traces", status: http.StatusUnauthorized},
		{method: http.MethodGet, path: "/apps/example/traces", status: http.StatusOK},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(test.method, test.path, nil)
		router.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Errorf("%s %s status = %d, want %d", test.method, test.path, response.Code, test.status)
		}
		if test.path != "/apps/example/traces" && strings.Contains(response.Body.String(), "id=app") {
			t.Errorf("%s %s unexpectedly served the SPA", test.method, test.path)
		}
	}
}
