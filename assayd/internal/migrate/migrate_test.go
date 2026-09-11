package migrate

import (
	"database/sql"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/db/migrations"
	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/store"
	"github.com/marioweid/assay/assayd/internal/testutil"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

func TestTraceMigrationIsEmbedded(t *testing.T) {
	if _, err := migrations.Files.ReadFile("00003_otlp_ingestion.sql"); err != nil {
		t.Fatalf("read embedded trace migration: %v", err)
	}
}

func TestOfflineScoringMigrationIsEmbedded(t *testing.T) {
	if _, err := migrations.Files.ReadFile("00004_offline_scoring.sql"); err != nil {
		t.Fatalf("read embedded offline scoring migration: %v", err)
	}
}

func TestOnlineScoringMigrationIsEmbedded(t *testing.T) {
	if _, err := migrations.Files.ReadFile("00005_online_scoring_generation.sql"); err != nil {
		t.Fatalf("read embedded online scoring migration: %v", err)
	}
}

func TestEvalRunSnapshotMigrationIsEmbedded(t *testing.T) {
	if _, err := migrations.Files.ReadFile("00006_eval_run_item_snapshots.sql"); err != nil {
		t.Fatalf("read embedded snapshot migration: %v", err)
	}
}

func TestUpAppliesMigrationAndIsIdempotent(t *testing.T) {
	database := openDatabase(t, testutil.Postgres(t))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := Up(t.Context(), database.MigrationDB(), logger); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if version := migrationVersion(t, database); version != 6 {
		t.Fatalf("migration version = %d, want 6", version)
	}
	assertScoringTables(t, database)
	if err := Up(t.Context(), database.MigrationDB(), logger); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
	if version := migrationVersion(t, database); version != 6 {
		t.Fatalf("migration version after reapply = %d, want 6", version)
	}
}

func TestEvalRunSnapshotMigrationBackfillsProcessableHistory(t *testing.T) {
	database := openDatabase(t, testutil.Postgres(t))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	provider := newTestMigrationProvider(t, database.MigrationDB(), logger)
	if _, err := provider.UpTo(t.Context(), 5); err != nil {
		t.Fatalf("apply migrations through version 5: %v", err)
	}
	fixture := seedVersionFiveRun(t, database.MigrationDB())

	if err := Up(t.Context(), database.MigrationDB(), logger); err != nil {
		t.Fatalf("apply snapshot migration: %v", err)
	}
	assertMigratedRunItem(t, database, fixture, "legacy_backfill")
	processMigratedRun(t, database, fixture)
}

func newTestMigrationProvider(
	t *testing.T,
	database *sql.DB,
	logger *slog.Logger,
) *goose.Provider {
	t.Helper()
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		database,
		migrations.Files,
		goose.WithSlog(logger),
	)
	if err != nil {
		t.Fatalf("create migration provider: %v", err)
	}
	return provider
}

func processMigratedRun(t *testing.T, database *store.Database, fixture versionFiveRun) {
	t.Helper()
	job, lease := startMigratedRun(t, database, fixture.runID)
	assertMigratedWorkerInput(t, database, fixture.runID)
	deleteMigratedSourceItem(t, database, fixture.itemID)
	assertMigratedRunItem(t, database, fixture, "legacy_backfill")
	finishMigratedRun(t, database, fixture, job.ID, lease)
}

func startMigratedRun(
	t *testing.T,
	database *store.Database,
	runID uuid.UUID,
) (domain.Job, domain.JobLease) {
	t.Helper()
	job, err := database.ClaimJob(t.Context(), "migration-worker", 30*time.Second)
	if err != nil {
		t.Fatalf("claim migrated run job: %v", err)
	}
	lease := domain.JobLease{JobID: job.ID, WorkerID: "migration-worker"}
	if _, err := database.StartEvalRun(t.Context(), runID, lease); err != nil {
		t.Fatalf("start migrated run: %v", err)
	}
	return job, lease
}

func assertMigratedWorkerInput(t *testing.T, database *store.Database, runID uuid.UUID) {
	t.Helper()
	items, err := database.ListPendingEvalRunItems(t.Context(), runID)
	if err != nil || len(items) != 1 || items[0].Item.Input["question"] != "legacy question" {
		t.Fatalf("migrated worker input = %#v, %v", items, err)
	}
}

func deleteMigratedSourceItem(t *testing.T, database *store.Database, itemID uuid.UUID) {
	t.Helper()
	if _, err := database.MigrationDB().ExecContext(
		t.Context(), "DELETE FROM dataset_items WHERE id = $1", itemID,
	); err != nil {
		t.Fatalf("delete migrated source item: %v", err)
	}
}

func finishMigratedRun(
	t *testing.T,
	database *store.Database,
	fixture versionFiveRun,
	jobID uuid.UUID,
	lease domain.JobLease,
) {
	t.Helper()
	if err := database.MarkEvalRunItemRunning(
		t.Context(), fixture.runID, fixture.itemID, lease,
	); err != nil {
		t.Fatalf("start migrated run item: %v", err)
	}
	if err := database.FailEvalRunItem(
		t.Context(), fixture.runID, fixture.itemID, "fixture complete", lease,
	); err != nil {
		t.Fatalf("finish migrated run item: %v", err)
	}
	if err := database.CompleteJob(t.Context(), jobID, lease.WorkerID); err != nil {
		t.Fatalf("complete migrated run job: %v", err)
	}
	run, err := database.GetEvalRun(t.Context(), fixture.runID)
	if err != nil || run.Status != domain.EvalStatusFailed {
		t.Fatalf("processed migrated run = %#v, %v", run, err)
	}
}

type versionFiveRun struct {
	runID  uuid.UUID
	itemID uuid.UUID
}

func seedVersionFiveRun(t *testing.T, database *sql.DB) versionFiveRun {
	t.Helper()
	fixture := versionFiveRun{runID: uuid.Must(uuid.NewV7()), itemID: uuid.Must(uuid.NewV7())}
	projectID := uuid.Must(uuid.NewV7())
	applicationID := uuid.Must(uuid.NewV7())
	datasetID := uuid.Must(uuid.NewV7())
	jobID := uuid.Must(uuid.NewV7())
	tx, err := database.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin version 5 fixture: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	statements := []struct {
		query string
		args  []any
	}{
		{
			"INSERT INTO projects (id, name) VALUES ($1, 'migration-project')",
			[]any{projectID},
		},
		{
			`INSERT INTO applications (id, project_id, name, slug)
			 VALUES ($1, $2, 'Migration', 'migration')`,
			[]any{applicationID, projectID},
		},
		{
			"INSERT INTO datasets (id, application_id, name) VALUES ($1, $2, 'legacy')",
			[]any{datasetID, applicationID},
		},
		{
			`INSERT INTO dataset_items (id, dataset_id, external_id, input, output, context)
			 VALUES ($1, $2, 'legacy-case', '{"question":"legacy question"}', 'answer', '[]')`,
			[]any{fixture.itemID, datasetID},
		},
		{
			`INSERT INTO eval_runs (
			    id, application_id, dataset_id, name, status, mode, scorers, total_items
			 ) VALUES (
			    $1, $2, $3, 'legacy-run', 'pending', 'score_existing', ARRAY['groundedness'], 1
			 )`,
			[]any{fixture.runID, applicationID, datasetID},
		},
		{
			`INSERT INTO eval_run_items (eval_run_id, dataset_item_id, status)
			 VALUES ($1, $2, 'pending')`,
			[]any{fixture.runID, fixture.itemID},
		},
		{
			`INSERT INTO jobs (id, kind, eval_run_id, status, max_attempts)
			 VALUES ($1, 'eval_run', $2, 'pending', 3)`,
			[]any{jobID, fixture.runID},
		},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(t.Context(), statement.query, statement.args...); err != nil {
			t.Fatalf("seed version 5 run: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit version 5 fixture: %v", err)
	}
	return fixture
}

func assertMigratedRunItem(
	t *testing.T,
	database *store.Database,
	fixture versionFiveRun,
	origin string,
) {
	t.Helper()
	var actualOrigin string
	var input string
	if err := database.MigrationDB().QueryRowContext(
		t.Context(),
		`SELECT snapshot_origin, snapshot_input->>'question'
		 FROM eval_run_items WHERE eval_run_id = $1 AND dataset_item_id = $2`,
		fixture.runID,
		fixture.itemID,
	).Scan(&actualOrigin, &input); err != nil {
		t.Fatalf("read migrated run item: %v", err)
	}
	if actualOrigin != origin || input != "legacy question" {
		t.Fatalf("migrated run item = origin %q, input %q", actualOrigin, input)
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
	if version := migrationVersion(t, first); version != 6 {
		t.Fatalf("migration version = %d, want 6", version)
	}
}

func assertScoringTables(t *testing.T, database *store.Database) {
	t.Helper()
	for _, table := range []string{
		"datasets", "dataset_items", "scorer_configs", "eval_runs",
		"eval_run_items", "scores", "scores_default", "jobs",
	} {
		var exists bool
		const query = `SELECT to_regclass($1) IS NOT NULL`
		if err := database.MigrationDB().QueryRowContext(
			t.Context(), query, table,
		).Scan(&exists); err != nil {
			t.Fatalf("inspect table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("table %s does not exist", table)
		}
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
