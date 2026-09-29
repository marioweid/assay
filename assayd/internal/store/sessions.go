package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/marioweid/assay/assayd/internal/domain"
	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListSessions returns a page of root-tagged sessions within one project and application.
func (d *Database) ListSessions(
	ctx context.Context, projectID uuid.UUID, query domain.SessionQuery,
) ([]domain.Session, error) {
	params := db.ListProjectSessionsParams{
		ProjectID: projectID, ApplicationID: query.ApplicationID,
		AnchorTime: timestamp(query.Anchor), PageSize: int32(query.Limit),
	}
	if query.Cursor != nil {
		params.HasCursor = true
		params.CursorTime = timestamp(query.Cursor.Time)
		params.CursorID = query.Cursor.ID
	}
	rows, err := d.queries.ListProjectSessions(ctx, params)
	if err != nil {
		return nil, mapStoreError("list project sessions", err)
	}
	items := make([]domain.Session, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.Session{
			ID: row.SessionID, StartTime: row.StartTime.Time, EndTime: row.EndTime.Time,
			TurnCount: row.TurnCount, FirstOperation: row.FirstOperation,
			LastTraceID: row.LastTraceID,
		})
	}
	return items, nil
}

// ListRecentSessionTurns bounds model context without walking every page in a long session.
func (d *Database) ListRecentSessionTurns(
	ctx context.Context, projectID uuid.UUID, query domain.SessionQuery,
) ([]domain.SessionTurn, error) {
	rows, err := d.queries.ListRecentSessionTurns(ctx, db.ListRecentSessionTurnsParams{
		ProjectID: projectID, ApplicationID: query.ApplicationID,
		SessionID: pgtype.Text{String: query.SessionID, Valid: true},
		PageSize:  int32(query.Limit),
	})
	if err != nil {
		return nil, mapStoreError("list recent session turns", err)
	}
	items := make([]domain.SessionTurn, 0, len(rows))
	for index := len(rows) - 1; index >= 0; index-- {
		row := rows[index]
		attributes := make(map[string]any)
		if err := json.Unmarshal(row.Attributes, &attributes); err != nil {
			return nil, fmt.Errorf("decode recent session root attributes: %w", err)
		}
		items = append(items, domain.SessionTurn{
			ID: row.ID, RootName: row.RootName,
			StartTime: row.StartTime.Time, EndTime: row.EndTime.Time,
			Status: row.Status, SpanCount: int(row.SpanCount), TotalTokens: row.TotalTokens,
			Attributes: attributes,
		})
	}
	return items, nil
}

// ListSessionTurns returns only root attributes, with no child span history.
func (d *Database) ListSessionTurns(
	ctx context.Context, projectID uuid.UUID, query domain.SessionQuery,
) ([]domain.SessionTurn, error) {
	var rows []db.ListProjectSessionTurnsRow
	var err error
	if query.Cursor == nil {
		rows, err = d.queries.ListProjectSessionTurns(ctx, db.ListProjectSessionTurnsParams{
			ProjectID: projectID, ApplicationID: query.ApplicationID,
			SessionID: pgtype.Text{String: query.SessionID, Valid: true},
			PageSize:  int32(query.Limit),
		})
	} else {
		var after []db.ListProjectSessionTurnsAfterCursorRow
		after, err = d.queries.ListProjectSessionTurnsAfterCursor(
			ctx, db.ListProjectSessionTurnsAfterCursorParams{
				ProjectID: projectID, ApplicationID: query.ApplicationID,
				SessionID:  pgtype.Text{String: query.SessionID, Valid: true},
				CursorTime: timestamp(query.Cursor.Time), CursorID: uuid.MustParse(query.Cursor.ID),
				PageSize: int32(query.Limit),
			},
		)
		for _, row := range after {
			rows = append(rows, db.ListProjectSessionTurnsRow(row))
		}
	}
	if err != nil {
		return nil, mapStoreError("list project session turns", err)
	}
	items := make([]domain.SessionTurn, 0, len(rows))
	for _, row := range rows {
		attributes := make(map[string]any)
		if err := json.Unmarshal(row.Attributes, &attributes); err != nil {
			return nil, fmt.Errorf("decode session turn root attributes: %w", err)
		}
		items = append(items, domain.SessionTurn{
			ID: row.ID, RootName: row.RootName,
			StartTime: row.StartTime.Time, EndTime: row.EndTime.Time,
			Status: row.Status, SpanCount: int(row.SpanCount), TotalTokens: row.TotalTokens,
			Attributes: attributes,
		})
	}
	return items, nil
}
