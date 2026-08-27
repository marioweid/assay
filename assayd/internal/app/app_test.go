package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/app"
	"github.com/marioweid/assay/assayd/internal/config"
	"github.com/marioweid/assay/assayd/internal/testutil"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestAppMigratesBeforeServingAndStops(t *testing.T) {
	dsn := testutil.Postgres(t)
	addr := unusedAddress(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application, err := app.New(t.Context(), config.Config{
		HTTPAddr:      addr,
		DatabaseURL:   dsn,
		AdminToken:    "admin-secret",
		EncryptionKey: config.EncryptionKey{},
	}, logger)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Errorf("close app: %v", err)
		}
	})

	if version := migrationVersion(t, dsn); version != 2 {
		t.Fatalf("migration version before serving = %d, want 2", version)
	}

	serveCtx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		result <- application.Serve(serveCtx)
	}()
	waitUntilReady(t, "http://"+addr+"/readyz")
	verifyM1Flow(t, "http://"+addr)

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serve after cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("app did not stop within five seconds")
	}
}

type apiRequest struct {
	method string
	path   string
	body   string
	token  string
}

func verifyM1Flow(t *testing.T, baseURL string) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	response := sendAPIRequest(t, client, baseURL, apiRequest{
		method: http.MethodPost,
		path:   "/v1/projects",
		body:   `{"name":"Support"}`,
	})
	status := response.StatusCode
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close unauthenticated response: %v", err)
	}
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated project status = %d, want 401", status)
	}

	project := sendAndDecode[struct {
		ID string `json:"id"`
	}](t, client, baseURL, apiRequest{
		method: http.MethodPost,
		path:   "/v1/projects",
		body:   `{"name":"Support"}`,
		token:  "admin-secret",
	}, http.StatusCreated)
	createdKey := sendAndDecode[struct {
		Key string `json:"key"`
	}](t, client, baseURL, apiRequest{
		method: http.MethodPost,
		path:   "/v1/projects/" + project.ID + "/keys",
		body:   `{"name":"CI"}`,
		token:  "admin-secret",
	}, http.StatusCreated)
	if !strings.HasPrefix(createdKey.Key, "asy_") {
		t.Fatalf("created key = %q, want asy_ prefix", createdKey.Key)
	}
	application := sendAndDecode[struct {
		ID string `json:"id"`
	}](t, client, baseURL, apiRequest{
		method: http.MethodPost,
		path:   "/v1/applications",
		body: `{"project_id":"` + project.ID + `","name":"Support Bot",` +
			`"slug":"support-bot"}`,
		token: "admin-secret",
	}, http.StatusCreated)
	if application.ID == "" {
		t.Fatal("created application ID is empty")
	}
}

func sendAndDecode[T any](
	t *testing.T,
	client *http.Client,
	baseURL string,
	request apiRequest,
	wantStatus int,
) T {
	t.Helper()
	response := sendAPIRequest(t, client, baseURL, request)
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close %s %s response: %v", request.method, request.path, err)
		}
	}()
	if response.StatusCode != wantStatus {
		t.Fatalf(
			"%s %s status = %d, want %d",
			request.method,
			request.path,
			response.StatusCode,
			wantStatus,
		)
	}
	var output T
	if err := json.NewDecoder(response.Body).Decode(&output); err != nil {
		t.Fatalf("decode %s %s response: %v", request.method, request.path, err)
	}
	return output
}

func sendAPIRequest(
	t *testing.T,
	client *http.Client,
	baseURL string,
	spec apiRequest,
) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(
		t.Context(),
		spec.method,
		baseURL+spec.path,
		strings.NewReader(spec.body),
	)
	if err != nil {
		t.Fatalf("create %s %s request: %v", spec.method, spec.path, err)
	}
	if spec.body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if spec.token != "" {
		request.Header.Set("Authorization", "Bearer "+spec.token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("send %s %s request: %v", spec.method, spec.path, err)
	}
	return response
}

func migrationVersion(t *testing.T, dsn string) int64 {
	t.Helper()
	connectionConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse migration verification DSN: %v", err)
	}
	database := stdlib.OpenDB(*connectionConfig)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close migration verification database: %v", err)
		}
	})
	return readMigrationVersion(t, database)
}

func readMigrationVersion(t *testing.T, database *sql.DB) int64 {
	t.Helper()
	const versionQuery = `SELECT max(version_id) FROM goose_db_version WHERE is_applied`
	var version int64
	if err := database.QueryRowContext(t.Context(), versionQuery).Scan(&version); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	return version
}

func unusedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve HTTP address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release HTTP address: %v", err)
	}
	return address
}

func waitUntilReady(t *testing.T, endpoint string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: time.Second}

	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			t.Fatalf("create readiness request: %v", err)
		}
		response, err := client.Do(request)
		if err == nil {
			closeErr := response.Body.Close()
			if response.StatusCode == http.StatusOK && closeErr == nil {
				return
			}
		}

		select {
		case <-ctx.Done():
			t.Fatalf("wait for app readiness: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}
