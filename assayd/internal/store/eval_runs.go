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

// CreateEvalRun atomically persists a run, its item outcomes, and its durable job.
func (d *Database) CreateEvalRun(
	ctx context.Context,
	run domain.EvalRun,
	job domain.Job,
) (domain.EvalRun, error) {
	if job.MaxAttempts <= 0 || int64(job.MaxAttempts) > int64(1<<31-1) {
		return domain.EvalRun{}, fmt.Errorf(
			"create eval run: %w: invalid job max attempts", domain.ErrInvalid,
		)
	}
	params, err := encodeJSON("eval run params", run.Params)
	if err != nil {
		return domain.EvalRun{}, err
	}
	return d.createEvalRunTransaction(ctx, run, job, params)
}

func (d *Database) createEvalRunTransaction(
	ctx context.Context,
	run domain.EvalRun,
	job domain.Job,
	params []byte,
) (domain.EvalRun, error) {
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return domain.EvalRun{}, fmt.Errorf("begin eval run transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	if err := queries.LockJobTableForWrite(ctx); err != nil {
		return domain.EvalRun{}, mapStoreError("lock jobs for run creation", err)
	}
	row, err := queries.CreateEvalRun(ctx, db.CreateEvalRunParams{
		ID: run.ID, Name: run.Name, Params: params, Scorers: run.Scorers,
		DatasetID: run.DatasetID, ApplicationID: run.ApplicationID,
	})
	if err != nil {
		return domain.EvalRun{}, mapStoreError("insert eval run", err)
	}
	if err := queries.CreateEvalRunItems(ctx, db.CreateEvalRunItemsParams{
		EvalRunID: run.ID, DatasetID: run.DatasetID,
	}); err != nil {
		return domain.EvalRun{}, mapStoreError("insert eval run items", err)
	}
	if _, err := queries.CreateEvalRunJob(ctx, db.CreateEvalRunJobParams{
		ID: job.ID, EvalRunID: run.ID, MaxAttempts: int32(job.MaxAttempts),
	}); err != nil {
		return domain.EvalRun{}, mapStoreError("insert eval run job", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.EvalRun{}, fmt.Errorf("commit eval run transaction: %w", err)
	}
	return evalRunFromRow(row)
}

func evalRunFromRow(row db.EvalRun) (domain.EvalRun, error) {
	run := domain.EvalRun{
		ID: row.ID, ApplicationID: row.ApplicationID, DatasetID: row.DatasetID,
		Name: row.Name, Status: row.Status, Mode: row.Mode, Scorers: row.Scorers,
		TotalItems: int(row.TotalItems), SucceededItems: int(row.SucceededItems),
		FailedItems: int(row.FailedItems), CanceledItems: int(row.CanceledItems),
		StartedAt: optionalTimestamp(row.StartedAt), FinishedAt: optionalTimestamp(row.FinishedAt),
		Error: optionalText(row.Error), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if err := decodeStoredJSON(row.Params, &run.Params); err != nil {
		return domain.EvalRun{}, fmt.Errorf("decode eval run params: %w", err)
	}
	if len(row.Aggregates) > 0 {
		if err := decodeStoredJSON(row.Aggregates, &run.Aggregates); err != nil {
			return domain.EvalRun{}, fmt.Errorf("decode eval run aggregates: %w", err)
		}
	}
	return run, nil
}

// ListEvalRuns returns cursor-paginated runs matching optional filters.
func (d *Database) ListEvalRuns(
	ctx context.Context,
	query domain.EvalRunQuery,
) ([]domain.EvalRun, error) {
	params := db.ListEvalRunsParams{PageSize: int32(query.Limit)}
	if query.ApplicationID != nil {
		params.FilterApplication = true
		params.ApplicationID = *query.ApplicationID
	}
	if query.Status != "" {
		params.FilterStatus = true
		params.Status = query.Status
	}
	if query.Cursor != nil {
		params.HasCursor = true
		params.CursorTime = timestamp(query.Cursor.CreatedAt)
		params.CursorID = query.Cursor.ID
	}
	rows, err := d.queries.ListEvalRuns(ctx, params)
	if err != nil {
		return nil, mapStoreError("select eval runs", err)
	}
	runs := make([]domain.EvalRun, 0, len(rows))
	for _, row := range rows {
		run, convertErr := evalRunFromRow(row)
		if convertErr != nil {
			return nil, convertErr
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// GetEvalRun returns one evaluation run by ID.
func (d *Database) GetEvalRun(ctx context.Context, runID uuid.UUID) (domain.EvalRun, error) {
	row, err := d.queries.GetEvalRun(ctx, runID)
	if err != nil {
		return domain.EvalRun{}, mapStoreError("select eval run", err)
	}
	return evalRunFromRow(row)
}

// CancelEvalRun cancels an active run, its pending items, and its job.
func (d *Database) CancelEvalRun(ctx context.Context, runID uuid.UUID) (domain.EvalRun, error) {
	row, err := d.queries.CancelEvalRun(ctx, runID)
	if err != nil {
		mappedErr := mapStoreError("cancel eval run", err)
		if !errors.Is(mappedErr, domain.ErrNotFound) {
			return domain.EvalRun{}, mappedErr
		}
		existing, lookupErr := d.queries.GetEvalRun(ctx, runID)
		if lookupErr != nil {
			return domain.EvalRun{}, mapStoreError("select eval run after cancel", lookupErr)
		}
		if existing.Status == domain.EvalStatusPending || existing.Status == domain.EvalStatusRunning {
			return domain.EvalRun{}, errors.New("cancel active eval run: durable job is missing")
		}
		return domain.EvalRun{}, fmt.Errorf("cancel eval run: %w: run is terminal", domain.ErrConflict)
	}
	return evalRunFromRow(db.EvalRun(row))
}

// ListEvalRunItems returns item-level run outcomes with their dataset inputs.
func (d *Database) ListEvalRunItems(
	ctx context.Context,
	runID uuid.UUID,
	query domain.PageQuery,
) ([]domain.EvalRunItem, error) {
	params := db.ListEvalRunItemsParams{EvalRunID: runID, PageSize: int32(query.Limit)}
	if query.Cursor != nil {
		params.HasCursor = true
		params.CursorTime = timestamp(query.Cursor.CreatedAt)
		params.CursorID = query.Cursor.ID
	}
	rows, err := d.queries.ListEvalRunItems(ctx, params)
	if err != nil {
		return nil, mapStoreError("select eval run items", err)
	}
	items := make([]domain.EvalRunItem, 0, len(rows))
	for _, row := range rows {
		item, convertErr := evalRunItemFromRow(row)
		if convertErr != nil {
			return nil, convertErr
		}
		items = append(items, item)
	}
	return items, nil
}

// ListEvalRunScores returns score rows using a bigint cursor.
func (d *Database) ListEvalRunScores(
	ctx context.Context,
	runID uuid.UUID,
	query domain.ScoreQuery,
) ([]domain.Score, error) {
	params := db.ListEvalRunScoresParams{EvalRunID: runID, PageSize: int32(query.Limit)}
	if query.Cursor != nil {
		params.HasCursor = true
		params.CursorTime = timestamp(query.Cursor.CreatedAt)
		params.CursorID = query.Cursor.ID
	}
	rows, err := d.queries.ListEvalRunScores(ctx, params)
	if err != nil {
		return nil, mapStoreError("select eval run scores", err)
	}
	scores := make([]domain.Score, 0, len(rows))
	for _, row := range rows {
		score, convertErr := scoreFromRow(row)
		if convertErr != nil {
			return nil, convertErr
		}
		scores = append(scores, score)
	}
	return scores, nil
}

func evalRunItemFromRow(row db.ListEvalRunItemsRow) (domain.EvalRunItem, error) {
	datasetItem, err := datasetItemFromRow(db.DatasetItem{
		ID: row.DatasetItemID, DatasetID: row.DatasetID, ExternalID: row.ExternalID,
		Input: row.Input, Output: row.Output, ExpectedOutput: row.ExpectedOutput,
		Context: row.Context, Metadata: row.Metadata, CreatedAt: row.ItemCreatedAt,
		UpdatedAt: row.ItemUpdatedAt,
	})
	if err != nil {
		return domain.EvalRunItem{}, err
	}
	return domain.EvalRunItem{
		EvalRunID: row.EvalRunID, DatasetItemID: row.DatasetItemID, Status: row.Status,
		Error: optionalText(row.Error), StartedAt: optionalTimestamp(row.StartedAt),
		FinishedAt: optionalTimestamp(row.FinishedAt), CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time, Item: datasetItem,
	}, nil
}

func scoreFromRow(row db.Score) (domain.Score, error) {
	value, err := numericFloat(row.Value)
	if err != nil {
		return domain.Score{}, fmt.Errorf("decode score value: %w", err)
	}
	threshold, err := numericFloat(row.Threshold)
	if err != nil {
		return domain.Score{}, fmt.Errorf("decode score threshold: %w", err)
	}
	score := domain.Score{
		ID: row.ID, Scorer: row.Scorer, Value: value, Threshold: threshold,
		Passed: row.Passed, Rationale: row.Rationale, PromptTemplateID: row.PromptTemplateID,
		JudgeModel: row.JudgeModel, JudgeProvider: row.JudgeProvider,
		JudgeTokens: int(row.JudgeTokens), EvalRunID: row.EvalRunID,
		DatasetItemID: row.DatasetItemID, CreatedAt: row.CreatedAt.Time,
	}
	if row.ScorerConfigID.Valid {
		configID := uuid.UUID(row.ScorerConfigID.Bytes)
		score.ScorerConfigID = &configID
	}
	if err := decodeStoredJSON(row.Details, &score.Details); err != nil {
		return domain.Score{}, fmt.Errorf("decode score details: %w", err)
	}
	return score, nil
}
