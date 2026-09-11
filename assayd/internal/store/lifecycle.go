package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/marioweid/assay/assayd/internal/domain"
	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// DeleteTrace removes a trace after fencing concurrent job writers.
func (d *Database) DeleteTrace(ctx context.Context, traceID uuid.UUID) error {
	return d.deleteWithJobLock(
		ctx, "delete trace", lockTraceJobs(traceID), func(queries *db.Queries) error {
			_, err := queries.DeleteTrace(ctx, traceID)
			return err
		})
}

// DeleteEvalRun removes a terminal run after fencing concurrent job writers.
func (d *Database) DeleteEvalRun(ctx context.Context, runID uuid.UUID) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete eval run transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	if err := lockEvalRunJobs(runID)(ctx, queries); err != nil {
		return mapStoreError("lock jobs for run deletion", err)
	}
	if _, err := queries.DeleteTerminalEvalRun(ctx, runID); err != nil {
		return classifyEvalRunDelete(ctx, queries, runID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete eval run transaction: %w", err)
	}
	return nil
}

func classifyEvalRunDelete(
	ctx context.Context,
	queries *db.Queries,
	runID uuid.UUID,
	deleteErr error,
) error {
	if !errors.Is(deleteErr, pgx.ErrNoRows) {
		return mapStoreError("delete eval run", deleteErr)
	}
	if _, err := queries.GetEvalRun(ctx, runID); err != nil {
		return mapStoreError("delete eval run", err)
	}
	return fmt.Errorf("delete eval run: %w: run is active", domain.ErrConflict)
}
