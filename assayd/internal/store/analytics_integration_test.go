package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marioweid/assay/assayd/internal/domain"
)

func TestAnalyticsScopesAggregatesAndPaginates(t *testing.T) {
	database := openTraceDatabase(t)
	service, traces := newTraceServices(t, database)
	project, application := createTraceApplication(t, service, "Analytics")
	trace := traceFixture(application.ID)
	if err := traces.Ingest(t.Context(), project.ID, []domain.Trace{trace}); err != nil {
		t.Fatal(err)
	}
	_, err := database.MigrationDB().ExecContext(t.Context(), `
 INSERT INTO scores (scorer, value, threshold, passed, rationale, prompt_template_id,
 judge_model, judge_provider, trace_id, span_id, span_start_time, judged_input,
 judged_output, judged_context, created_at)
 SELECT 'groundedness', value, 0.5, value >= 0.5, 'evidence', 'v1', 'fake', 'fake',
 $1, 1, '2026-08-28T12:00:00Z', 'question', 'answer', '[]', created_at::timestamptz
 FROM (VALUES (0.2, '2026-09-01T01:00:00Z'), (0.8, '2026-09-01T02:00:00Z'),
 (1.0, '2026-09-02T00:00:00Z')) AS fixture(value, created_at)`, trace.ID)
	requireStoreSuccess(t, err)
	analytics := domain.NewAnalyticsService(database)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	query := domain.AnalyticsQuery{
		ApplicationID: application.ID, Start: start, End: start.AddDate(0, 0, 1),
	}
	points, err := analytics.Metrics(t.Context(), query)
	requireStoreSuccess(t, err)
	assertStoreValue(t, "point count", len(points), 1)
	assertStoreValue(t, "score count", points[0].N, 2)
	assertStoreValue(t, "mean", points[0].Mean, 0.5)
	assertStoreValue(t, "pass rate", points[0].PassRate, 0.5)
	query.Limit = 1
	first, err := analytics.Scores(t.Context(), query)
	requireStoreSuccess(t, err)
	assertStoreValue(t, "first page size", len(first.Items), 1)
	assertStoreValue(t, "first value", first.Items[0].Value, 0.2)
	assertStoreValue(t, "first cursor exists", first.NextCursor != nil, true)
	query.Cursor = first.NextCursor
	second, err := analytics.Scores(t.Context(), query)
	requireStoreSuccess(t, err)
	assertStoreValue(t, "second page size", len(second.Items), 1)
	assertStoreValue(t, "second value", second.Items[0].Value, 0.8)
	assertStoreValue(t, "second cursor absent", second.NextCursor == nil, true)
	query.Cursor = nil
	failed := false
	query.Passed = &failed
	filtered, err := analytics.Scores(t.Context(), query)
	requireStoreSuccess(t, err)
	assertStoreValue(t, "filtered count", len(filtered.Items), 1)
	assertStoreValue(t, "filtered passed", filtered.Items[0].Passed, false)
	_, other := createTraceApplication(t, service, "Other analytics")
	query.ApplicationID = other.ID
	points, err = analytics.Metrics(t.Context(), query)
	if err != nil || len(points) != 0 {
		t.Fatalf("other metrics = %+v, %v", points, err)
	}
	query.ApplicationID = uuid.New()
	if _, err := analytics.Metrics(t.Context(), query); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing application error = %v", err)
	}
}

func requireStoreSuccess(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func assertStoreValue[T comparable](t *testing.T, name string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}
