package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/marioweid/assay/assayd/internal/domain"
)

type metricsInput struct {
	ID     string    `path:"id" format:"uuid"`
	Start  time.Time `query:"start" required:"false"`
	End    time.Time `query:"end" required:"false"`
	Scorer string    `query:"scorer" required:"false" enum:"groundedness,correctness"`
}

type applicationScoresInput struct {
	ApplicationID string    `query:"application_id" format:"uuid" required:"true"`
	Start         time.Time `query:"start" required:"false"`
	End           time.Time `query:"end" required:"false"`
	Scorer        string    `query:"scorer" required:"false" enum:"groundedness,correctness"`
	Passed        string    `query:"passed" required:"false" enum:"true,false"`
	Limit         int       `query:"limit" minimum:"0" maximum:"500"`
	Cursor        string    `query:"cursor" required:"false"`
}

type metricsResult struct {
	Body struct {
		Items []domain.MetricPoint `json:"items"`
	}
}

func (h *handler) registerAnalyticsRoutes() {
	huma.Register(h.api, h.operation(http.MethodGet, "/v1/applications/{id}/metrics",
		"application-metrics", "Read daily application score trends", http.StatusNotFound), h.metrics)
	huma.Register(h.api, h.operation(http.MethodGet, "/v1/scores",
		"list-application-scores", "Filter application scores", http.StatusNotFound), h.applicationScores)
}

func (h *handler) metrics(ctx context.Context, input *metricsInput) (*metricsResult, error) {
	points, err := h.analytics.Metrics(ctx, domain.AnalyticsQuery{
		ApplicationID: uuid.MustParse(input.ID), Start: input.Start, End: input.End, Scorer: input.Scorer,
	})
	if err != nil {
		return nil, h.responseError("query analytics", err)
	}
	result := &metricsResult{}
	result.Body.Items = points
	return result, nil
}

func (h *handler) applicationScores(
	ctx context.Context, input *applicationScoresInput,
) (*scoreCollectionResult, error) {
	cursor, err := decodeScoreCursor(input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid score cursor", err)
	}
	var passed *bool
	if input.Passed != "" {
		value := input.Passed == "true"
		passed = &value
	}
	page, err := h.analytics.Scores(ctx, domain.AnalyticsQuery{
		ApplicationID: uuid.MustParse(input.ApplicationID), Start: input.Start, End: input.End,
		Scorer: input.Scorer, Passed: passed,
		ScoreQuery: domain.ScoreQuery{Limit: input.Limit, Cursor: cursor},
	})
	if err != nil {
		return nil, h.responseError("query analytics", err)
	}
	result := &scoreCollectionResult{}
	result.Body.Items = make([]scoreResponse, 0, len(page.Items))
	for _, score := range page.Items {
		result.Body.Items = append(result.Body.Items, scoreOutput(score))
	}
	result.Body.NextCursor, err = encodeScoreCursor(page.NextCursor)
	if err != nil {
		return nil, h.responseError("query analytics", err)
	}
	return result, nil
}
