package store_test

import (
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
)

func TestRetentionMovesDefaultRowsAndPreservesScores(t *testing.T) {
	database := openTraceDatabase(t)
	service, traces := newTraceServices(t, database)
	project, application := createTraceApplication(t, service, "Retention")
	trace := traceFixture(application.ID)
	requireStoreSuccess(t, traces.Ingest(t.Context(), project.ID, []domain.Trace{trace}))
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	requireStoreSuccess(t, database.MaintainPartitions(t.Context(), now, 0))
	var partition string
	err := database.MigrationDB().QueryRowContext(t.Context(),
		`SELECT tableoid::regclass::text FROM spans LIMIT 1`).Scan(&partition)
	requireStoreSuccess(t, err)
	assertStoreValue(t, "partition", partition, "spans_202608")
	_, err = database.MigrationDB().ExecContext(t.Context(), `
 INSERT INTO scores (scorer, value, threshold, passed, rationale, prompt_template_id,
 judge_model, judge_provider, trace_id, span_id, span_start_time, judged_input,
 judged_output, judged_context) VALUES ('groundedness', 0.2, 0.5, false, 'evidence',
 'v1', 'fake', 'fake', $1, 1, '2026-08-28T12:00:00Z', 'question', 'answer', '[]')`, trace.ID)
	requireStoreSuccess(t, err)
	cutoffNow := time.Date(2026, 8, 29, 12, 0, 1, 0, time.UTC)
	requireStoreSuccess(t, database.MaintainPartitions(t.Context(), cutoffNow, 1))
	detail, err := traces.Get(t.Context(), project.ID, trace.ID)
	requireStoreSuccess(t, err)
	assertStoreValue(t, "boundary spans", len(detail.Spans), 1)
	assertStoreValue(t, "retained scores", len(detail.Scores), 1)
	assertStoreValue(t, "evidence", detail.Scores[0].JudgedOutput, "answer")
	for range 2 {
		requireStoreSuccess(t, database.MaintainPartitions(t.Context(), now.AddDate(0, 0, 1), 1))
	}
	detail, err = traces.Get(t.Context(), project.ID, trace.ID)
	requireStoreSuccess(t, err)
	assertStoreValue(t, "pruned spans", len(detail.Spans), 0)
	assertStoreValue(t, "surviving scores", len(detail.Scores), 1)
	var exists bool
	err = database.MigrationDB().QueryRowContext(t.Context(),
		`SELECT to_regclass('spans_202608') IS NOT NULL`).Scan(&exists)
	if err != nil || exists {
		t.Fatalf("expired partition exists = %v, %v", exists, err)
	}
}

func TestConcurrentRetentionPreservesUnexpiredSpans(t *testing.T) {
	database := openTraceDatabase(t)
	service, traces := newTraceServices(t, database)
	project, application := createTraceApplication(t, service, "Concurrent retention")
	trace := traceFixture(application.ID)
	requireStoreSuccess(t, traces.Ingest(t.Context(), project.ID, []domain.Trace{trace}))
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			results <- database.MaintainPartitions(t.Context(), now, 1)
		}()
	}
	close(start)
	for range 2 {
		requireStoreSuccess(t, <-results)
	}
	detail, err := traces.Get(t.Context(), project.ID, trace.ID)
	requireStoreSuccess(t, err)
	assertStoreValue(t, "unexpired spans", len(detail.Spans), len(trace.Spans))
}
