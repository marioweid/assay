package app_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/app"
	"github.com/marioweid/assay/assayd/internal/config"
	"github.com/marioweid/assay/assayd/internal/testutil"
)

func TestLocalAppWiresGuardAndAnonymousManagement(t *testing.T) {
	dsn := testutil.Postgres(t)
	addr := unusedAddress(t)
	application, err := app.New(t.Context(), config.Config{
		HTTPAddr: addr, DatabaseURL: dsn, LocalMode: true,
		WorkerConcurrency: 1, JobMaxAttempts: 3,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- application.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-result:
			if err != nil {
				t.Errorf("stop local app: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("local app did not stop")
		}
		if err := application.Close(); err != nil {
			t.Errorf("close local app: %v", err)
		}
	})
	waitUntilReady(t, "http://"+addr+"/readyz")
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	for _, test := range []struct {
		name, method, path, host, origin, key string
		status                                int
	}{
		{"discover", "GET", "/v1/server-info", "", "", "", http.StatusOK},
		{"create", "POST", "/v1/projects", "", "", "", http.StatusCreated},
		{"read", "GET", "/v1/projects", "", "", "", http.StatusOK},
		{"rebound", "GET", "/v1/projects", "attacker.test", "", "", http.StatusForbidden},
		{"cross-origin", "POST", "/v1/projects", "", "https://attacker.test", "",
			http.StatusForbidden},
		{"bad key", "GET", "/v1/projects", "", "", "invalid", http.StatusUnauthorized},
		{"ingest", "POST", "/v1/traces", "", "", "", http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(test.method, "http://"+addr+test.path,
				strings.NewReader(`{"name":"Local test"}`))
			if err != nil {
				t.Fatal(err)
			}
			if test.host != "" {
				request.Host = test.host
			}
			request.Header.Set("Origin", test.origin)
			request.Header.Set("X-API-Key", test.key)
			request.Header.Set("Content-Type", "application/json")
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != test.status {
				t.Errorf("status=%d, want %d", response.StatusCode, test.status)
			}
		})
	}
}
