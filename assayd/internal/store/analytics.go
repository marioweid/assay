package store

import (
	"context"

	"github.com/marioweid/assay/assayd/internal/domain"
	db "github.com/marioweid/assay/assayd/internal/store/sqlc"
)

// ApplicationMetrics aggregates persisted scores by UTC day and scorer.
func (d *Database) ApplicationMetrics(
	ctx context.Context, query domain.AnalyticsQuery,
) ([]domain.MetricPoint, error) {
	rows, err := d.queries.ApplicationMetrics(ctx, db.ApplicationMetricsParams{
		ApplicationID: query.ApplicationID, StartTime: timestamp(query.Start),
		EndTime: timestamp(query.End), Scorer: query.Scorer,
	})
	if err != nil {
		return nil, mapStoreError("aggregate application scores", err)
	}
	points := make([]domain.MetricPoint, 0, len(rows))
	for _, row := range rows {
		points = append(points, domain.MetricPoint{Date: row.Day.Time, Scorer: row.Scorer,
			ScoreAggregate: domain.ScoreAggregate{N: int(row.N), Mean: row.Mean, PassRate: row.PassRate}})
	}
	return points, nil
}

// ListApplicationScores reads a filtered score page including denormalized evidence.
func (d *Database) ListApplicationScores(
	ctx context.Context, query domain.AnalyticsQuery,
) ([]domain.Score, error) {
	params := db.ListApplicationScoresParams{
		ApplicationID: query.ApplicationID, StartTime: timestamp(query.Start),
		EndTime: timestamp(query.End), Scorer: query.Scorer, PageSize: int32(query.Limit),
	}
	if query.Passed != nil {
		params.FilterPassed = true
		params.Passed = *query.Passed
	}
	if query.Cursor != nil {
		params.HasCursor = true
		params.CursorTime = timestamp(query.Cursor.CreatedAt)
		params.CursorID = query.Cursor.ID
	}
	rows, err := d.queries.ListApplicationScores(ctx, params)
	if err != nil {
		return nil, mapStoreError("select application scores", err)
	}
	scores := make([]domain.Score, 0, len(rows))
	for _, row := range rows {
		score, err := scoreFromRow(row)
		if err != nil {
			return nil, err
		}
		scores = append(scores, score)
	}
	return scores, nil
}
