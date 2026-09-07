package domain

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AnalyticsQuery scopes score reads to an application and a half-open time range.
type AnalyticsQuery struct {
	ApplicationID uuid.UUID
	Start         time.Time
	End           time.Time
	Scorer        string
	Passed        *bool
	ScoreQuery
}

// MetricPoint summarizes scores for one UTC day and scorer.
type MetricPoint struct {
	Date   time.Time `json:"date"`
	Scorer string    `json:"scorer"`
	ScoreAggregate
}

// AnalyticsRepository supplies application-scoped score reads and aggregates.
type AnalyticsRepository interface {
	GetApplication(context.Context, uuid.UUID) (Application, error)
	ApplicationMetrics(context.Context, AnalyticsQuery) ([]MetricPoint, error)
	ListApplicationScores(context.Context, AnalyticsQuery) ([]Score, error)
}

// AnalyticsService validates score queries before reading persisted results.
type AnalyticsService struct{ repository AnalyticsRepository }

// NewAnalyticsService constructs application score reporting.
func NewAnalyticsService(repository AnalyticsRepository) *AnalyticsService {
	return &AnalyticsService{repository: repository}
}

// Metrics returns daily UTC aggregates using each score's recorded pass decision.
func (s *AnalyticsService) Metrics(
	ctx context.Context, query AnalyticsQuery,
) ([]MetricPoint, error) {
	if err := s.validate(ctx, &query); err != nil {
		return nil, err
	}
	points, err := s.repository.ApplicationMetrics(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("read application metrics: %w", err)
	}
	return points, nil
}

// Scores returns one stable ascending page of filtered application scores.
func (s *AnalyticsService) Scores(ctx context.Context, query AnalyticsQuery) (ScorePage, error) {
	if err := s.validate(ctx, &query); err != nil {
		return ScorePage{}, err
	}
	if query.Limit == 0 {
		query.Limit = 100
	}
	if query.Limit < 1 || query.Limit > 500 {
		return ScorePage{}, fmt.Errorf("list scores: %w: limit must be between 1 and 500", ErrInvalid)
	}
	size := query.Limit
	query.Limit++
	scores, err := s.repository.ListApplicationScores(ctx, query)
	if err != nil {
		return ScorePage{}, fmt.Errorf("list application scores: %w", err)
	}
	page := ScorePage{Items: scores}
	if len(scores) > size {
		page.Items = scores[:size]
		last := page.Items[size-1]
		page.NextCursor = &ScoreCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

func (s *AnalyticsService) validate(ctx context.Context, query *AnalyticsQuery) error {
	if query.ApplicationID == uuid.Nil {
		return fmt.Errorf("query scores: %w: application_id is required", ErrInvalid)
	}
	if query.Scorer != "" && !knownScorer(query.Scorer) {
		return fmt.Errorf("query scores: %w: unknown scorer", ErrInvalid)
	}
	if err := normalizeAnalyticsRange(query); err != nil {
		return err
	}
	if _, err := s.repository.GetApplication(ctx, query.ApplicationID); err != nil {
		return fmt.Errorf("query score application: %w", err)
	}
	return nil
}

func normalizeAnalyticsRange(query *AnalyticsQuery) error {
	if query.End.IsZero() {
		query.End = time.Now().UTC()
	}
	if query.Start.IsZero() {
		query.Start = query.End.AddDate(0, 0, -30)
	}
	if !query.Start.Before(query.End) || query.End.Sub(query.Start) > 366*24*time.Hour {
		return fmt.Errorf("query scores: %w: range must be positive and at most 366 days", ErrInvalid)
	}
	return nil
}

func knownScorer(scorer string) bool {
	return scorer == ScorerGroundedness || scorer == ScorerCorrectness
}
