package migrate

import (
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/marioweid/assay/assayd/internal/store"
	"github.com/marioweid/assay/assayd/internal/testutil"
)

func TestUpAppliesMigrationAndIsIdempotent(t *testing.T) {
	database := openDatabase(t, testutil.Postgres(t))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := Up(t.Context(), database.MigrationDB(), logger); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if version := migrationVersion(t, database); version != 2 {
		t.Fatalf("migration version = %d, want 2", version)
	}
	if err := Up(t.Context(), database.MigrationDB(), logger); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
	if version := migrationVersion(t, database); version != 2 {
		t.Fatalf("migration version after reapply = %d, want 2", version)
	}
}

func TestUpSerializesConcurrentReplicas(t *testing.T) {
	dsn := testutil.Postgres(t)
	first := openDatabase(t, dsn)
	second := openDatabase(t, dsn)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	start := make(chan struct{})
	errors := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, database := range []*store.Database{first, second} {
		go func() {
			ready.Done()
			<-start
			errors <- Up(t.Context(), database.MigrationDB(), logger)
		}()
	}
	ready.Wait()
	close(start)

	for range 2 {
		if err := <-errors; err != nil {
			t.Errorf("concurrent migration: %v", err)
		}
	}
	if version := migrationVersion(t, first); version != 2 {
		t.Fatalf("migration version = %d, want 2", version)
	}
}

func openDatabase(t *testing.T, dsn string) *store.Database {
	t.Helper()
	database, err := store.Open(t.Context(), dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	return database
}

func migrationVersion(t *testing.T, database *store.Database) int64 {
	t.Helper()
	const query = `SELECT max(version_id) FROM goose_db_version WHERE is_applied`
	var version int64
	if err := database.MigrationDB().QueryRowContext(t.Context(), query).Scan(&version); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	return version
}
