package domain_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestResolveApplicationUsesProjectSlug(t *testing.T) {
	projectID := uuid.Must(uuid.NewV7())
	want := domain.Application{ID: uuid.Must(uuid.NewV7()), ProjectID: projectID, Slug: "support"}
	repository := &traceRepositoryFake{application: want}
	service := domain.NewTraceService(repository, &applicationCreatorFake{}, 3)

	got, err := service.ResolveApplication(t.Context(), projectID, " support ", false)
	if err != nil {
		t.Fatalf("resolve application: %v", err)
	}
	if got.ID != want.ID || repository.projectID != projectID || repository.slug != "support" {
		t.Fatalf("resolved application = %#v, repository = %#v", got, repository)
	}
}

func TestResolveApplicationAutoCreatesUnknownSlug(t *testing.T) {
	projectID := uuid.Must(uuid.NewV7())
	created := domain.Application{ID: uuid.Must(uuid.NewV7()), ProjectID: projectID, Slug: "support"}
	repository := &traceRepositoryFake{applicationErr: domain.ErrNotFound}
	creator := &applicationCreatorFake{application: created}
	service := domain.NewTraceService(repository, creator, 3)

	got, err := service.ResolveApplication(t.Context(), projectID, "support", true)
	if err != nil {
		t.Fatalf("auto-create application: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("application ID = %s, want %s", got.ID, created.ID)
	}
	if creator.input.Name != "support" || creator.input.Slug != "support" {
		t.Fatalf("create input = %#v", creator.input)
	}
}

func TestResolveApplicationRejectsMissingOrUnknownSlug(t *testing.T) {
	service := domain.NewTraceService(
		&traceRepositoryFake{applicationErr: domain.ErrNotFound},
		&applicationCreatorFake{},
		3,
	)
	projectID := uuid.Must(uuid.NewV7())

	if _, err := service.ResolveApplication(t.Context(), projectID, " ", false); !errors.Is(
		err,
		domain.ErrInvalid,
	) {
		t.Fatalf("blank slug error = %v, want ErrInvalid", err)
	}
	if _, err := service.ResolveApplication(t.Context(), projectID, "missing", false); !errors.Is(
		err,
		domain.ErrNotFound,
	) {
		t.Fatalf("unknown slug error = %v, want ErrNotFound", err)
	}
}

func TestListTracesPaginates(t *testing.T) {
	projectID := uuid.Must(uuid.NewV7())
	start := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	items := make([]domain.Trace, 3)
	for index := range items {
		items[index] = domain.Trace{
			ID:        uuid.Must(uuid.NewV7()),
			StartTime: start.Add(-time.Duration(index) * time.Minute),
		}
	}
	repository := &traceRepositoryFake{traces: items}
	service := domain.NewTraceService(repository, &applicationCreatorFake{}, 3)

	page, err := service.List(t.Context(), projectID, domain.TraceQuery{Limit: 2})
	if err != nil {
		t.Fatalf("list traces: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("trace page = %#v, want two items and cursor", page)
	}
	if page.NextCursor == nil || page.NextCursor.ID != items[1].ID {
		t.Fatalf("cursor/query = %#v/%#v", page.NextCursor, repository.query)
	}
	if repository.query.Limit != 3 {
		t.Fatalf("repository limit = %d, want 3", repository.query.Limit)
	}
}

func TestListTracesValidatesFilters(t *testing.T) {
	projectID := uuid.Must(uuid.NewV7())
	start := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	service := domain.NewTraceService(&traceRepositoryFake{}, &applicationCreatorFake{}, 3)
	end := start.Add(-time.Hour)
	if _, err := service.List(t.Context(), projectID, domain.TraceQuery{
		Start: &start,
		End:   &end,
	}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("inverted range error = %v, want ErrInvalid", err)
	}
	if _, err := service.List(t.Context(), projectID, domain.TraceQuery{Limit: 201}); !errors.Is(
		err,
		domain.ErrInvalid,
	) {
		t.Fatalf("oversized limit error = %v, want ErrInvalid", err)
	}
}

func TestIngestCreatesUniqueEligibleAutomaticScoreIntents(t *testing.T) {
	applicationID := uuid.Must(uuid.NewV7())
	trace := scoringTrace(applicationID)
	repository := &traceRepositoryFake{application: domain.Application{
		ID: applicationID, AutoScoreScorers: []string{domain.ScorerGroundedness},
	}}
	service := domain.NewTraceService(repository, &applicationCreatorFake{}, 4)
	err := service.Ingest(
		t.Context(), uuid.Must(uuid.NewV7()), []domain.Trace{trace, trace},
	)
	if err != nil {
		t.Fatalf("ingest traces: %v", err)
	}
	if len(repository.intents) != 1 || repository.intents[0].Scorer != domain.ScorerGroundedness ||
		repository.intents[0].MaxAttempts != 4 {
		t.Fatalf("automatic score intents = %#v", repository.intents)
	}
}

func TestQueueScoresValidatesBatchBeforePersistence(t *testing.T) {
	applicationID := uuid.Must(uuid.NewV7())
	trace := scoringTrace(applicationID)
	trace.ID = uuid.Must(uuid.NewV7())
	repository := &traceRepositoryFake{trace: trace}
	service := domain.NewTraceService(repository, &applicationCreatorFake{}, 3)
	jobs, err := service.QueueScores(
		t.Context(), uuid.Must(uuid.NewV7()), []uuid.UUID{trace.ID},
		[]string{domain.ScorerGroundedness}, true,
	)
	if err != nil {
		t.Fatalf("queue trace scores: %v", err)
	}
	if len(jobs) != 1 || !repository.refresh || len(repository.requests) != 1 {
		t.Fatalf("queued jobs/requests = %#v/%#v", jobs, repository.requests)
	}
	_, err = service.QueueScores(
		t.Context(), uuid.Must(uuid.NewV7()), []uuid.UUID{trace.ID, trace.ID},
		[]string{domain.ScorerGroundedness}, false,
	)
	if !errors.Is(err, domain.ErrInvalid) || repository.queueCalls != 1 {
		t.Fatalf("duplicate batch error/calls = %v/%d", err, repository.queueCalls)
	}
}

func TestAttachReferenceRefreshesAutomaticCorrectness(t *testing.T) {
	applicationID := uuid.Must(uuid.NewV7())
	trace := scoringTrace(applicationID)
	trace.ID = uuid.Must(uuid.NewV7())
	repository := &traceRepositoryFake{
		trace: trace,
		application: domain.Application{
			ID: applicationID, AutoScoreScorers: []string{domain.ScorerCorrectness},
		},
	}
	service := domain.NewTraceService(repository, &applicationCreatorFake{}, 5)
	_, err := service.AttachReference(t.Context(), uuid.Must(uuid.NewV7()), trace.ID, " expected ")
	if err != nil {
		t.Fatalf("attach reference: %v", err)
	}
	if repository.reference != "expected" || repository.referenceJob == nil ||
		repository.referenceJob.Scorer != domain.ScorerCorrectness {
		t.Fatalf("reference/job = %q/%#v", repository.reference, repository.referenceJob)
	}
}

func scoringTrace(applicationID uuid.UUID) domain.Trace {
	return domain.Trace{
		ApplicationID: applicationID, OTelTraceID: [16]byte{1},
		Spans: []domain.Span{{IsScorable: true, Attributes: map[string]any{
			"gen_ai.input.messages": []any{map[string]any{"role": "user", "content": "question"}},
			"gen_ai.output.messages": []any{
				map[string]any{"role": "assistant", "content": "answer"},
			},
			"gen_ai.retrieval.documents": []any{"context"},
		}}},
	}
}

type traceRepositoryFake struct {
	application    domain.Application
	applicationErr error
	projectID      uuid.UUID
	slug           string
	traces         []domain.Trace
	query          domain.TraceQuery
	ingested       []domain.Trace
	intents        []domain.AutoScoreIntent
	trace          domain.Trace
	requests       []domain.TraceScoreRequest
	refresh        bool
	queueCalls     int
	reference      string
	referenceJob   *domain.Job
}

func (f *traceRepositoryFake) GetApplicationByProjectSlug(
	_ context.Context,
	projectID uuid.UUID,
	slug string,
) (domain.Application, error) {
	f.projectID = projectID
	f.slug = slug
	return f.application, f.applicationErr
}

func (f *traceRepositoryFake) UpsertTraces(
	_ context.Context,
	projectID uuid.UUID,
	traces []domain.Trace,
	intents []domain.AutoScoreIntent,
) error {
	f.projectID = projectID
	f.ingested = traces
	f.intents = intents
	return nil
}

func (f *traceRepositoryFake) GetApplication(
	_ context.Context,
	_ uuid.UUID,
) (domain.Application, error) {
	return f.application, f.applicationErr
}

func (f *traceRepositoryFake) ListScorerConfigs(
	context.Context,
	uuid.UUID,
) ([]domain.ScorerConfig, error) {
	return nil, nil
}

func (f *traceRepositoryFake) ListTraces(
	_ context.Context,
	projectID uuid.UUID,
	query domain.TraceQuery,
) ([]domain.Trace, error) {
	f.projectID = projectID
	f.query = query
	return f.traces, nil
}

func (f *traceRepositoryFake) GetTrace(
	_ context.Context,
	projectID uuid.UUID,
	_ uuid.UUID,
) (domain.Trace, error) {
	f.projectID = projectID
	return f.trace, nil
}

func (f *traceRepositoryFake) QueueTraceScores(
	_ context.Context,
	_ uuid.UUID,
	requests []domain.TraceScoreRequest,
	refresh bool,
) ([]domain.Job, error) {
	f.requests = requests
	f.refresh = refresh
	f.queueCalls++
	jobs := make([]domain.Job, 0, len(requests))
	for _, request := range requests {
		traceID := request.TraceID
		jobs = append(jobs, domain.Job{
			ID: request.JobID, TraceID: &traceID, Scorer: request.Scorer,
		})
	}
	return jobs, nil
}

func (f *traceRepositoryFake) AttachTraceReference(
	_ context.Context,
	_ uuid.UUID,
	_ uuid.UUID,
	reference string,
	job *domain.Job,
) (domain.Trace, error) {
	f.reference = reference
	f.referenceJob = job
	f.trace.ReferenceAnswer = &reference
	return f.trace, nil
}

type applicationCreatorFake struct {
	application domain.Application
	err         error
	input       domain.CreateApplicationInput
}

func (f *applicationCreatorFake) CreateApplication(
	_ context.Context,
	input domain.CreateApplicationInput,
) (domain.Application, error) {
	f.input = input
	return f.application, f.err
}
