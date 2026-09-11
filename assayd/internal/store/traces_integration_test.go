package store_test

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	secretcrypto "github.com/marioweid/assay/assayd/internal/crypto"
	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/migrate"
	"github.com/marioweid/assay/assayd/internal/store"
	"github.com/marioweid/assay/assayd/internal/testutil"

	"github.com/google/uuid"
)

func TestTraceStoreUpsertsIncrementalExportsAndScopesReads(t *testing.T) {
	database := openTraceDatabase(t)
	service, traceService := newTraceServices(t, database)
	project, application := createTraceApplication(t, service, "Primary")
	otherProject, otherApplication := createTraceApplication(t, service, "Other")
	trace := traceFixture(application.ID)
	ingestTraceFixtures(t, traceService, project.ID, trace)
	assertStoredTrace(t, traceService, project.ID, otherProject.ID)
	assertSpanOwnership(t, traceService, project.ID, application.ID, otherApplication.ID)
}

func assertSpanOwnership(
	t *testing.T,
	service *domain.TraceService,
	projectID uuid.UUID,
	applicationID uuid.UUID,
	otherApplicationID uuid.UUID,
) {
	t.Helper()
	trace := traceFixture(applicationID)
	trace.ID = uuid.Must(uuid.NewV7())
	trace.OTelTraceID = [16]byte{9}
	trace.Spans = trace.Spans[:1]
	trace.Spans[0].ApplicationID = otherApplicationID
	if err := service.Ingest(t.Context(), projectID, []domain.Trace{trace}); err == nil {
		t.Fatal("ingest span owned by another application succeeded")
	}
}

func newTraceServices(
	t *testing.T,
	database *store.Database,
) (*domain.Service, *domain.TraceService) {
	t.Helper()
	cipher, err := secretcrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	service := domain.NewService(database, cipher)
	return service, domain.NewTraceService(database, service, 3)
}

func traceFixture(applicationID uuid.UUID) domain.Trace {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	traceID := [16]byte{1, 2, 3}
	rootID := [8]byte{1}
	childID := [8]byte{2}
	return domain.Trace{
		ID:            uuid.Must(uuid.NewV7()),
		ApplicationID: applicationID,
		OTelTraceID:   traceID,
		RootName:      "answer",
		StartTime:     now,
		EndTime:       now.Add(2 * time.Second),
		Status:        "ok",
		Attributes:    map[string]any{"service.name": "support"},
		Spans: []domain.Span{
			{
				ApplicationID: applicationID,
				OTelSpanID:    rootID,
				Name:          "answer",
				StartTime:     now,
				EndTime:       now.Add(2 * time.Second),
				StatusCode:    "ok",
				Attributes:    map[string]any{"root": true},
				Events:        []domain.SpanEvent{},
			},
			{
				ApplicationID: applicationID,
				OTelSpanID:    childID,
				ParentSpanID:  &rootID,
				Name:          "generation",
				StartTime:     now.Add(time.Second),
				EndTime:       now.Add(2 * time.Second),
				StatusCode:    "ok",
				InputTokens:   10,
				OutputTokens:  5,
				Attributes:    map[string]any{},
				Events:        []domain.SpanEvent{},
			},
		},
	}
}

func ingestTraceFixtures(
	t *testing.T,
	service *domain.TraceService,
	projectID uuid.UUID,
	trace domain.Trace,
) {
	t.Helper()
	if err := service.Ingest(t.Context(), projectID, []domain.Trace{trace, trace}); err != nil {
		t.Fatalf("ingest duplicate trace export: %v", err)
	}
	rootID := trace.Spans[0].OTelSpanID
	now := trace.StartTime
	incremental := trace
	incremental.ID = uuid.Must(uuid.NewV7())
	incremental.Spans = []domain.Span{{
		ApplicationID: trace.ApplicationID,
		OTelSpanID:    [8]byte{3},
		ParentSpanID:  &rootID,
		Name:          "tool",
		StartTime:     now.Add(1500 * time.Millisecond),
		EndTime:       now.Add(1700 * time.Millisecond),
		StatusCode:    "ok",
		Attributes:    map[string]any{},
		Events:        []domain.SpanEvent{},
	}}
	if err := service.Ingest(t.Context(), projectID, []domain.Trace{incremental}); err != nil {
		t.Fatalf("ingest incremental trace export: %v", err)
	}
}

func assertStoredTrace(
	t *testing.T,
	service *domain.TraceService,
	projectID uuid.UUID,
	otherProjectID uuid.UUID,
) {
	t.Helper()
	page, err := service.List(t.Context(), projectID, domain.TraceQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list traces: %v", err)
	}
	assertTracePage(t, page)
	detail, err := service.Get(t.Context(), projectID, page.Items[0].ID)
	if err != nil {
		t.Fatalf("get trace detail: %v", err)
	}
	assertTraceDetail(t, detail)
	if _, err := service.Get(t.Context(), otherProjectID, detail.ID); !errors.Is(
		err,
		domain.ErrNotFound,
	) {
		t.Fatalf("cross-project trace error = %v, want not found", err)
	}
}

func assertTracePage(t *testing.T, page domain.TracePage) {
	t.Helper()
	if len(page.Items) != 1 {
		t.Fatalf("trace summary = %#v", page.Items)
	}
	if page.Items[0].SpanCount != 3 || page.Items[0].TotalTokens != 15 {
		t.Fatalf("trace summary = %#v", page.Items[0])
	}
}

func assertTraceDetail(t *testing.T, detail domain.Trace) {
	t.Helper()
	if len(detail.Spans) != 3 {
		t.Fatalf("trace detail = %#v", detail)
	}
	if detail.RootName != "answer" {
		t.Fatalf("trace root = %q, want answer", detail.RootName)
	}
}

//nolint:cyclop // The queue-to-persistence regression verifies the full correctness path.
func TestCorrectnessOnlyTraceQueuesAndPersistsEmptyContext(t *testing.T) {
	database := openTraceDatabase(t)
	service, traces := newTraceServices(t, database)
	project, application := createTraceApplication(t, service, "Correctness")
	reference := "expected"
	trace := traceFixture(application.ID)
	trace.ReferenceAnswer = &reference
	trace.Spans = trace.Spans[:1]
	trace.Spans[0].IsScorable = true
	trace.Spans[0].ReferenceAnswer = &reference
	trace.Spans[0].Attributes = map[string]any{
		"gen_ai.input.messages":  []any{map[string]any{"role": "user", "content": "question"}},
		"gen_ai.output.messages": []any{map[string]any{"role": "assistant", "content": "answer"}},
	}
	if err := traces.Ingest(t.Context(), project.ID, []domain.Trace{trace}); err != nil {
		t.Fatalf("ingest correctness trace: %v", err)
	}
	cipher, err := secretcrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	scores := domain.NewTraceServiceWithScorerResolver(
		database, service, domain.NewEvaluationService(database, cipher, 3),
		domain.JudgeDefaults{BaseURL: "http://judge.test", Model: "test"}, 3,
	)
	jobs, err := scores.QueueScores(
		t.Context(), project.ID, []uuid.UUID{trace.ID}, []string{domain.ScorerCorrectness}, true,
	)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("queue correctness score = %#v, %v", jobs, err)
	}
	job, err := database.ClaimJob(t.Context(), "correctness-worker", 30*time.Second)
	if err != nil {
		t.Fatalf("claim correctness score: %v", err)
	}
	stored, err := database.GetTraceByID(t.Context(), trace.ID)
	if err != nil {
		t.Fatalf("load correctness trace: %v", err)
	}
	input, err := domain.BuildTraceScoreInput(stored, domain.ScorerCorrectness)
	if err != nil || input.Context == nil {
		t.Fatalf("build correctness score input = %#v, %v", input, err)
	}
	traceID, spanID, spanStart := stored.ID, input.Span.ID, input.Span.StartTime
	if err := database.CompleteTraceScore(t.Context(), domain.Score{
		Scorer: domain.ScorerCorrectness, Value: 1, Threshold: 0.5, Passed: true,
		Rationale: "correct", Details: map[string]any{}, PromptTemplateID: domain.CorrectnessPromptV1,
		JudgeModel: "test", JudgeProvider: "test", TraceID: &traceID, SpanID: &spanID,
		SpanStartTime: &spanStart, JudgedInput: input.Input, JudgedOutput: input.Output,
		JudgedContext: input.Context, JudgedReference: &input.Reference,
	}, domain.JobLease{JobID: job.ID, WorkerID: "correctness-worker"}); err != nil {
		t.Fatalf("persist correctness score: %v", err)
	}
	var contextType string
	if err := database.MigrationDB().QueryRowContext(
		t.Context(), `SELECT jsonb_typeof(judged_context) FROM scores WHERE trace_id = $1`, trace.ID,
	).Scan(&contextType); err != nil || contextType != "array" {
		t.Fatalf("stored correctness context type = %q, %v; want array", contextType, err)
	}
}

type traceDiscoveryFixture struct {
	first, second domain.Trace
	project       domain.Project
	traces        *domain.TraceService
}

func TestTraceStoreSearchesBeforePaginationAndLoadsScoreSummaries(t *testing.T) {
	fixture := newTraceDiscoveryFixture(t)
	assertLiteralSearchAndPagination(t, fixture)
	assertScoreFilterAndSummaries(t, fixture)
	assertTraceIDSearch(t, fixture)
}

func newTraceDiscoveryFixture(t *testing.T) traceDiscoveryFixture {
	database := openTraceDatabase(t)
	service, traces := newTraceServices(t, database)
	project, application := createTraceApplication(t, service, "Primary")
	first := namedTrace(application.ID, "Needle%_\\", 1, 0)
	second := namedTrace(application.ID, "needle second", 2, -time.Minute)
	if err := traces.Ingest(t.Context(), project.ID, []domain.Trace{first, second}); err != nil {
		t.Fatalf("ingest searchable traces: %v", err)
	}
	stored, err := traces.Get(t.Context(), project.ID, first.ID)
	if err != nil {
		t.Fatalf("get scored trace: %v", err)
	}
	insertTraceScore(t, database, traceScoreInput{
		traceID: first.ID, span: stored.Spans[0], scorer: domain.ScorerCorrectness, passed: true,
	})
	insertTraceScore(t, database, traceScoreInput{
		traceID: first.ID, span: stored.Spans[0], scorer: domain.ScorerGroundedness, passed: true,
	})
	insertTraceScore(t, database, traceScoreInput{
		traceID: first.ID, span: stored.Spans[0], scorer: domain.ScorerGroundedness, passed: false,
	})
	return traceDiscoveryFixture{first: first, second: second, project: project, traces: traces}
}

func namedTrace(
	applicationID uuid.UUID,
	name string,
	otelID byte,
	offset time.Duration,
) domain.Trace {
	trace := traceFixture(applicationID)
	trace.ID = uuid.Must(uuid.NewV7())
	trace.OTelTraceID = [16]byte{otelID}
	trace.RootName = name
	for index := range trace.Spans {
		trace.Spans[index].Name = name
		trace.Spans[index].StartTime = trace.Spans[index].StartTime.Add(offset)
		trace.Spans[index].EndTime = trace.Spans[index].EndTime.Add(offset)
	}
	return trace
}

func assertLiteralSearchAndPagination(t *testing.T, fixture traceDiscoveryFixture) {
	t.Helper()
	page, err := fixture.traces.List(t.Context(), fixture.project.ID, domain.TraceQuery{
		Q: "needle%_\\", Limit: 1,
	})
	assertTracePageResult(t, page, err, fixture.first.ID, "literal search")
	page, err = fixture.traces.List(t.Context(), fixture.project.ID, domain.TraceQuery{
		Q: "needle", Limit: 1,
	})
	if page.NextCursor == nil {
		t.Fatalf("search first page has no cursor: %#v", page)
	}
	assertTracePageResult(t, page, err, fixture.first.ID, "search first page")
	page, err = fixture.traces.List(t.Context(), fixture.project.ID, domain.TraceQuery{
		Q: "needle", Limit: 1, Cursor: page.NextCursor,
	})
	assertTracePageResult(t, page, err, fixture.second.ID, "search second page")
}

func assertScoreFilterAndSummaries(t *testing.T, fixture traceDiscoveryFixture) {
	t.Helper()
	failed := false
	page, err := fixture.traces.List(t.Context(), fixture.project.ID, domain.TraceQuery{
		Scorer: domain.ScorerGroundedness, Passed: &failed, Limit: 2,
	})
	assertTracePageResult(t, page, err, fixture.first.ID, "score filter")
	summaries := page.Items[0].ScoreSummaries
	if len(summaries) != 2 || summaries[0].Scorer != domain.ScorerCorrectness ||
		summaries[1].Scorer != domain.ScorerGroundedness || summaries[1].Passed {
		t.Fatalf("score summaries = %#v", summaries)
	}
	passed := true
	page, err = fixture.traces.List(t.Context(), fixture.project.ID, domain.TraceQuery{
		Scorer: domain.ScorerGroundedness, Passed: &passed, Limit: 2,
	})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("latest score filter = %#v, %v; want no traces", page, err)
	}
}

func assertTraceIDSearch(t *testing.T, fixture traceDiscoveryFixture) {
	t.Helper()
	for _, q := range []string{fixture.first.ID.String(), "01000000000000000000000000000000"} {
		page, err := fixture.traces.List(t.Context(), fixture.project.ID, domain.TraceQuery{
			Q: q, Limit: 2,
		})
		assertTracePageResult(t, page, err, fixture.first.ID, "ID search "+q)
	}
}

func assertTracePageResult(
	t *testing.T,
	page domain.TracePage,
	err error,
	want uuid.UUID,
	operation string,
) {
	t.Helper()
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != want {
		t.Fatalf("%s = %#v, %v", operation, page, err)
	}
}

type traceScoreInput struct {
	traceID uuid.UUID
	span    domain.Span
	scorer  string
	passed  bool
}

func insertTraceScore(t *testing.T, database *store.Database, input traceScoreInput) {
	t.Helper()
	_, err := database.MigrationDB().ExecContext(t.Context(), `
INSERT INTO scores (
    scorer, value, threshold, passed, rationale, prompt_template_id, judge_model, judge_provider,
    trace_id, span_id, span_start_time, judged_input, judged_output, judged_context
) VALUES (
    $1, 0.8, 0.7, $2, 'test', 'test', 'test', 'test', $3, $4, $5, 'input', 'output',
    '[]'::jsonb
)`,
		input.scorer, input.passed, input.traceID, input.span.ID, input.span.StartTime)
	if err != nil {
		t.Fatalf("insert trace score: %v", err)
	}
}

func openTraceDatabase(t *testing.T) *store.Database {
	t.Helper()
	database, err := store.Open(t.Context(), testutil.Postgres(t))
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
	return database
}

func createTraceApplication(
	t *testing.T,
	service *domain.Service,
	name string,
) (domain.Project, domain.Application) {
	t.Helper()
	project, err := service.CreateProject(t.Context(), domain.CreateProjectInput{Name: name})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	application, err := service.CreateApplication(t.Context(), domain.CreateApplicationInput{
		ProjectID: project.ID,
		Name:      name,
		Slug:      name,
	})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	return project, application
}
