package app_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net"
	"net/http"
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
		HTTPAddr:    addr,
		DatabaseURL: dsn,
	}, logger)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Errorf("close app: %v", err)
		}
	})

	if version := migrationVersion(t, dsn); version != 1 {
		t.Fatalf("migration version before serving = %d, want 1", version)
	}

	serveCtx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		result <- application.Serve(serveCtx)
	}()
	waitUntilReady(t, "http://"+addr+"/readyz")

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serve after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("app did not stop within one second")
	}
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
