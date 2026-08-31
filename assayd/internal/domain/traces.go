package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/marioweid/assay/assayd/internal/id"

	"github.com/google/uuid"
)

const (
	defaultTraceLimit = 50
	maxTraceLimit     = 200
)

// TraceRepository persists and reads project-scoped trace data.
type TraceRepository interface {
	GetApplicationByProjectSlug(context.Context, uuid.UUID, string) (Application, error)
	GetApplication(context.Context, uuid.UUID) (Application, error)
	ListScorerConfigs(context.Context, uuid.UUID) ([]ScorerConfig, error)
	UpsertTraces(context.Context, uuid.UUID, []Trace, []AutoScoreIntent) error
	ListTraces(context.Context, uuid.UUID, TraceQuery) ([]Trace, error)
	GetTrace(context.Context, uuid.UUID, uuid.UUID) (Trace, error)
	QueueTraceScores(context.Context, uuid.UUID, []TraceScoreRequest, bool) ([]Job, error)
	AttachTraceReference(context.Context, uuid.UUID, uuid.UUID, string, *Job) (Trace, error)
}

type applicationCreator interface {
	CreateApplication(context.Context, CreateApplicationInput) (Application, error)
}

// TraceService owns application resolution, ingestion, and project-scoped trace reads.
type TraceService struct {
	repository  TraceRepository
	creator     applicationCreator
	maxAttempts int
}

// NewTraceService constructs trace workflows from focused persistence and application contracts.
func NewTraceService(
	repository TraceRepository,
	creator applicationCreator,
	maxAttempts int,
) *TraceService {
	return &TraceService{repository: repository, creator: creator, maxAttempts: maxAttempts}
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
	intents, err := s.autoScoreIntents(ctx, traces)
	if err != nil {
		return err
	}
	if err := s.repository.UpsertTraces(ctx, projectID, traces, intents); err != nil {
		return fmt.Errorf("ingest %d traces: %w", len(traces), err)
	}
	return nil
}

func (s *TraceService) autoScoreIntents(
	ctx context.Context,
	traces []Trace,
) ([]AutoScoreIntent, error) {
	settings := make(map[uuid.UUID]autoScoreSettings)
	seen := make(map[string]struct{})
	var intents []AutoScoreIntent
	for _, trace := range traces {
		setting, err := s.cachedAutoScoreSettings(ctx, trace.ApplicationID, settings)
		if err != nil {
			return nil, err
		}
		for _, scorer := range setting.application.AutoScoreScorers {
			key := trace.ApplicationID.String() + string(trace.OTelTraceID[:]) + scorer
			if _, duplicate := seen[key]; duplicate || !setting.configs[scorer].Enabled {
				continue
			}
			jobID, err := id.New()
			if err != nil {
				return nil, fmt.Errorf("generate automatic scoring task ID: %w", err)
			}
			seen[key] = struct{}{}
			intents = append(intents, AutoScoreIntent{
				ApplicationID: trace.ApplicationID, OTelTraceID: trace.OTelTraceID,
				Scorer: scorer, JobID: jobID, MaxAttempts: s.maxAttempts,
			})
		}
	}
	return intents, nil
}

type autoScoreSettings struct {
	application Application
	configs     map[string]ScorerConfig
}

func (s *TraceService) cachedAutoScoreSettings(
	ctx context.Context,
	applicationID uuid.UUID,
	cache map[uuid.UUID]autoScoreSettings,
) (autoScoreSettings, error) {
	if setting, found := cache[applicationID]; found {
		return setting, nil
	}
	application, err := s.repository.GetApplication(ctx, applicationID)
	if err != nil {
		return autoScoreSettings{}, fmt.Errorf("load automatic scoring application: %w", err)
	}
	if err := validateAutoScoreScorers(application.AutoScoreScorers); err != nil {
		return autoScoreSettings{}, fmt.Errorf("load automatic scoring application: %w", err)
	}
	persisted, err := s.repository.ListScorerConfigs(ctx, applicationID)
	if err != nil {
		return autoScoreSettings{}, fmt.Errorf("load automatic scorer configs: %w", err)
	}
	setting := autoScoreSettings{
		application: application, configs: effectiveConfigMap(applicationID, persisted),
	}
	cache[applicationID] = setting
	return setting, nil
}

// QueueScores validates and queues a project-scoped trace scoring batch atomically.
func (s *TraceService) QueueScores(
	ctx context.Context,
	projectID uuid.UUID,
	traceIDs []uuid.UUID,
	scorers []string,
	refresh bool,
) ([]Job, error) {
	if err := validateTraceScoreSelection(traceIDs, scorers); err != nil {
		return nil, err
	}
	requests := make([]TraceScoreRequest, 0, len(traceIDs)*len(scorers))
	for _, traceID := range traceIDs {
		trace, err := s.repository.GetTrace(ctx, projectID, traceID)
		if err != nil {
			return nil, fmt.Errorf("queue trace scores: %w", err)
		}
		if err := s.validateTraceScorers(ctx, trace, scorers); err != nil {
			return nil, err
		}
		for _, scorer := range scorers {
			jobID, err := id.New()
			if err != nil {
				return nil, fmt.Errorf("generate trace scoring task ID: %w", err)
			}
			requests = append(requests, TraceScoreRequest{
				TraceID: trace.ID, Scorer: scorer, JobID: jobID, MaxAttempts: s.maxAttempts,
			})
		}
	}
	jobs, err := s.repository.QueueTraceScores(ctx, projectID, requests, refresh)
	if err != nil {
		return nil, fmt.Errorf("queue trace scores: %w", err)
	}
	return jobs, nil
}

func validateTraceScoreSelection(traceIDs []uuid.UUID, scorers []string) error {
	if len(traceIDs) == 0 {
		return fmt.Errorf("queue trace scores: %w: at least one trace required", ErrInvalid)
	}
	seen := make(map[uuid.UUID]struct{}, len(traceIDs))
	for _, traceID := range traceIDs {
		if traceID == uuid.Nil {
			return fmt.Errorf("queue trace scores: %w: invalid trace ID", ErrInvalid)
		}
		if _, duplicate := seen[traceID]; duplicate {
			return fmt.Errorf("queue trace scores: %w: duplicate trace ID", ErrInvalid)
		}
		seen[traceID] = struct{}{}
	}
	return validateScorerNames(scorers)
}

func (s *TraceService) validateTraceScorers(
	ctx context.Context,
	trace Trace,
	scorers []string,
) error {
	persisted, err := s.repository.ListScorerConfigs(ctx, trace.ApplicationID)
	if err != nil {
		return fmt.Errorf("queue trace scores: list scorer configs: %w", err)
	}
	configs := effectiveConfigMap(trace.ApplicationID, persisted)
	for _, scorer := range scorers {
		if !configs[scorer].Enabled {
			return fmt.Errorf("queue trace scores: %w: scorer %q is disabled", ErrInvalid, scorer)
		}
		if _, err := BuildTraceScoreInput(trace, scorer); err != nil {
			return err
		}
	}
	return nil
}

// AttachReference stores a non-blank reference and refreshes automatic correctness when enabled.
func (s *TraceService) AttachReference(
	ctx context.Context,
	projectID uuid.UUID,
	traceID uuid.UUID,
	reference string,
) (Trace, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Trace{}, fmt.Errorf("attach trace reference: %w: reference is blank", ErrInvalid)
	}
	trace, err := s.repository.GetTrace(ctx, projectID, traceID)
	if err != nil {
		return Trace{}, fmt.Errorf("attach trace reference: %w", err)
	}
	application, err := s.repository.GetApplication(ctx, trace.ApplicationID)
	if err != nil {
		return Trace{}, fmt.Errorf("attach trace reference: %w", err)
	}
	job, err := s.referenceScoreJob(ctx, trace, application, reference)
	if err != nil {
		return Trace{}, err
	}
	trace, err = s.repository.AttachTraceReference(ctx, projectID, traceID, reference, job)
	if err != nil {
		return Trace{}, fmt.Errorf("attach trace reference: %w", err)
	}
	return trace, nil
}

func (s *TraceService) referenceScoreJob(
	ctx context.Context,
	trace Trace,
	application Application,
	reference string,
) (*Job, error) {
	if err := validateAutoScoreScorers(application.AutoScoreScorers); err != nil {
		return nil, fmt.Errorf("attach trace reference: %w", err)
	}
	configs, err := s.repository.ListScorerConfigs(ctx, trace.ApplicationID)
	if err != nil {
		return nil, fmt.Errorf("attach trace reference: list scorer configs: %w", err)
	}
	if !containsString(application.AutoScoreScorers, ScorerCorrectness) ||
		!effectiveConfigMap(trace.ApplicationID, configs)[ScorerCorrectness].Enabled {
		return nil, nil
	}
	trace.ReferenceAnswer = &reference
	if _, err := BuildTraceScoreInput(trace, ScorerCorrectness); err != nil {
		return nil, nil
	}
	jobID, err := id.New()
	if err != nil {
		return nil, fmt.Errorf("generate correctness task ID: %w", err)
	}
	traceID := trace.ID
	return &Job{
		ID: jobID, Kind: JobKindScoringTask, TraceID: &traceID,
		Scorer: ScorerCorrectness, MaxAttempts: s.maxAttempts,
	}, nil
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
