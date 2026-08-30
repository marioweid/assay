package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	defaultTraceLimit = 50
	maxTraceLimit     = 200
)

// TraceRepository persists and reads project-scoped trace data.
type TraceRepository interface {
	GetApplicationByProjectSlug(context.Context, uuid.UUID, string) (Application, error)
	UpsertTraces(context.Context, uuid.UUID, []Trace) error
	ListTraces(context.Context, uuid.UUID, TraceQuery) ([]Trace, error)
	GetTrace(context.Context, uuid.UUID, uuid.UUID) (Trace, error)
}

type applicationCreator interface {
	CreateApplication(context.Context, CreateApplicationInput) (Application, error)
}

// TraceService owns application resolution, ingestion, and project-scoped trace reads.
type TraceService struct {
	repository TraceRepository
	creator    applicationCreator
}

// NewTraceService constructs trace workflows from focused persistence and application contracts.
func NewTraceService(repository TraceRepository, creator applicationCreator) *TraceService {
	return &TraceService{repository: repository, creator: creator}
}

// ResolveApplication resolves or optionally creates a project application from a resource slug.
func (s *TraceService) ResolveApplication(
	ctx context.Context,
	projectID uuid.UUID,
	slug string,
	autoCreate bool,
) (Application, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return Application{}, fmt.Errorf("resolve trace application: %w: slug is blank", ErrInvalid)
	}
	application, err := s.repository.GetApplicationByProjectSlug(ctx, projectID, slug)
	if err == nil {
		return application, nil
	}
	if !errors.Is(err, ErrNotFound) || !autoCreate {
		return Application{}, fmt.Errorf("resolve trace application %q: %w", slug, err)
	}
	application, err = s.creator.CreateApplication(ctx, CreateApplicationInput{
		ProjectID: projectID,
		Name:      slug,
		Slug:      slug,
	})
	if errors.Is(err, ErrConflict) {
		application, err = s.repository.GetApplicationByProjectSlug(ctx, projectID, slug)
	}
	if err != nil {
		return Application{}, fmt.Errorf("auto-create trace application %q: %w", slug, err)
	}
	return application, nil
}

// Ingest persists all accepted traces in one repository operation.
func (s *TraceService) Ingest(
	ctx context.Context,
	projectID uuid.UUID,
	traces []Trace,
) error {
	if len(traces) == 0 {
		return nil
	}
	if err := s.repository.UpsertTraces(ctx, projectID, traces); err != nil {
		return fmt.Errorf("ingest %d traces: %w", len(traces), err)
	}
	return nil
}

// List validates filters and returns one project-scoped trace page.
func (s *TraceService) List(
	ctx context.Context,
	projectID uuid.UUID,
	query TraceQuery,
) (TracePage, error) {
	if err := normalizeTraceQuery(&query); err != nil {
		return TracePage{}, err
	}
	pageSize := query.Limit
	query.Limit++
	traces, err := s.repository.ListTraces(ctx, projectID, query)
	if err != nil {
		return TracePage{}, fmt.Errorf("list traces: %w", err)
	}
	page := TracePage{Items: traces}
	if len(traces) > pageSize {
		page.Items = traces[:pageSize]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = &TraceCursor{StartTime: last.StartTime, ID: last.ID}
	}
	return page, nil
}

// Get returns one trace and its spans when it belongs to the project.
func (s *TraceService) Get(
	ctx context.Context,
	projectID uuid.UUID,
	traceID uuid.UUID,
) (Trace, error) {
	trace, err := s.repository.GetTrace(ctx, projectID, traceID)
	if err != nil {
		return Trace{}, fmt.Errorf("get trace %s: %w", traceID, err)
	}
	return trace, nil
}

func normalizeTraceQuery(query *TraceQuery) error {
	if query.Limit == 0 {
		query.Limit = defaultTraceLimit
	}
	if query.Limit < 1 || query.Limit > maxTraceLimit {
		return fmt.Errorf("list traces: %w: limit must be between 1 and 200", ErrInvalid)
	}
	if query.Start != nil && query.End != nil && !query.Start.Before(*query.End) {
		return fmt.Errorf("list traces: %w: start must be before end", ErrInvalid)
	}
	query.Status = strings.TrimSpace(query.Status)
	return nil
}
