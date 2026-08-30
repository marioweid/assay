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
	service := domain.NewTraceService(repository, &applicationCreatorFake{})

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
	service := domain.NewTraceService(repository, creator)

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
	service := domain.NewTraceService(repository, &applicationCreatorFake{})

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
	service := domain.NewTraceService(&traceRepositoryFake{}, &applicationCreatorFake{})
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

type traceRepositoryFake struct {
	application    domain.Application
	applicationErr error
	projectID      uuid.UUID
	slug           string
	traces         []domain.Trace
	query          domain.TraceQuery
	ingested       []domain.Trace
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
) error {
	f.projectID = projectID
	f.ingested = traces
	return nil
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
	return domain.Trace{}, nil
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
