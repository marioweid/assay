package store

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/migrate"
	db "github.com/marioweid/assay/assayd/internal/store/sqlc"
	"github.com/marioweid/assay/assayd/internal/testutil"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestListTracesKeepsFilteredScoresInOneSnapshot(t *testing.T) {
	blocker := newScoreSummaryBlocker()
	database := openSnapshotTraceDatabase(t, blocker)
	projectID, traceID, spanID, scoreTime := createSnapshotTrace(t, database)
	failed := false
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	results := make(chan traceListResult, 1)
	go func() {
		traces, err := database.ListTraces(ctx, projectID, domain.TraceQuery{
			Scorer: domain.ScorerGroundedness, Passed: &failed, Limit: 2,
		})
		results <- traceListResult{traces: traces, err: err}
	}()
	defer blocker.Resume()

	select {
	case <-blocker.started:
	case <-ctx.Done():
		t.Fatalf("wait for trace score summary query: %v", ctx.Err())
	}
	insertSnapshotScore(t, database, snapshotScoreInput{
		traceID: traceID, spanID: spanID, createdAt: scoreTime.Add(time.Second), passed: true,
	})
	blocker.Resume()

	select {
	case result := <-results:
		assertSnapshotTraceResult(t, result)
	case <-ctx.Done():
		t.Fatalf("list traces: %v", ctx.Err())
	}
}

type traceListResult struct {
	traces []domain.Trace
	err    error
}

func assertSnapshotTraceResult(t *testing.T, result traceListResult) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("list filtered traces: %v", result.err)
	}
	if len(result.traces) != 1 || len(result.traces[0].ScoreSummaries) != 1 {
		t.Fatalf("filtered traces = %#v", result.traces)
	}
	if result.traces[0].ScoreSummaries[0].Passed {
		t.Fatalf("filtered trace returned a passing summary: %#v", result.traces[0])
	}
}

type scoreSummaryBlocker struct {
	started     chan struct{}
	resume      chan struct{}
	startedOnce sync.Once
	resumeOnce  sync.Once
}

func newScoreSummaryBlocker() *scoreSummaryBlocker {
	return &scoreSummaryBlocker{started: make(chan struct{}), resume: make(chan struct{})}
}

func (b *scoreSummaryBlocker) TraceQueryStart(
	ctx context.Context,
	_ *pgx.Conn,
	data pgx.TraceQueryStartData,
) context.Context {
	if strings.Contains(data.SQL, "SELECT DISTINCT ON (scores.trace_id, scores.scorer)") {
		b.startedOnce.Do(func() {
			close(b.started)
			select {
			case <-b.resume:
			case <-ctx.Done():
			}
		})
	}
	return ctx
}

func (*scoreSummaryBlocker) TraceQueryEnd(
	context.Context,
	*pgx.Conn,
	pgx.TraceQueryEndData,
) {
}

func (b *scoreSummaryBlocker) Resume() {
	b.resumeOnce.Do(func() { close(b.resume) })
}

func openSnapshotTraceDatabase(t *testing.T, tracer pgx.QueryTracer) *Database {
	t.Helper()
	dsn := testutil.Postgres(t)
	database, err := Open(t.Context(), dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrate.Up(t.Context(), database.MigrationDB(), logger); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	poolConfig.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(t.Context(), poolConfig)
	if err != nil {
		t.Fatalf("create traced database pool: %v", err)
	}
	database.pool.Close()
	database.pool = pool
	database.queries = db.New(pool)
	return database
}

func createSnapshotTrace(
	t *testing.T,
	database *Database,
) (uuid.UUID, uuid.UUID, int64, time.Time) {
	t.Helper()
	projectID := uuid.Must(uuid.NewV7())
	applicationID := uuid.Must(uuid.NewV7())
	traceID := uuid.Must(uuid.NewV7())
	scoreTime := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	mustExecSnapshot(t, database, "insert project", `
INSERT INTO projects (id, name) VALUES ($1, 'Snapshot test')`, projectID)
	mustExecSnapshot(t, database, "insert application", `
INSERT INTO applications (id, project_id, name, slug)
VALUES ($1, $2, 'Snapshot test', 'snapshot-test')`, applicationID, projectID)
	mustExecSnapshot(t, database, "insert trace", `
INSERT INTO traces (
    id, application_id, otel_trace_id, root_name, start_time, end_time, status, attributes
) VALUES ($1, $2, $3, 'snapshot', $4, $4, 'ok', '{}'::jsonb)`,
		traceID, applicationID, make([]byte, 16), scoreTime)
	var spanID int64
	err := database.MigrationDB().QueryRowContext(t.Context(), `
INSERT INTO spans (
    trace_id, application_id, otel_span_id, name, kind, start_time, end_time,
    duration_ms, status_code
) VALUES ($1, $2, $3, 'snapshot', 'internal', $4, $4, 0, 'ok')
RETURNING id`, traceID, applicationID, make([]byte, 8), scoreTime).Scan(&spanID)
	if err != nil {
		t.Fatalf("insert trace span: %v", err)
	}
	insertSnapshotScore(t, database, snapshotScoreInput{
		traceID: traceID, spanID: spanID, createdAt: scoreTime, passed: false,
	})
	return projectID, traceID, spanID, scoreTime
}

type snapshotScoreInput struct {
	traceID   uuid.UUID
	spanID    int64
	createdAt time.Time
	passed    bool
}

func insertSnapshotScore(t *testing.T, database *Database, input snapshotScoreInput) {
	t.Helper()
	mustExecSnapshot(t, database, "insert trace score", `
INSERT INTO scores (
    scorer, value, threshold, passed, rationale, prompt_template_id, judge_model, judge_provider,
    trace_id, span_id, span_start_time, judged_input, judged_output, judged_context, created_at
) VALUES (
    'groundedness', 0.8, 0.7, $1, 'test', 'test', 'test', 'test', $2, $3, $4, 'input', 'output',
    '[]'::jsonb, $4
)`, input.passed, input.traceID, input.spanID, input.createdAt)
}

func mustExecSnapshot(t *testing.T, database *Database, operation, query string, args ...any) {
	t.Helper()
	if _, err := database.MigrationDB().ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("%s: %v", operation, err)
	}
}

var _ pgx.QueryTracer = (*scoreSummaryBlocker)(nil)
