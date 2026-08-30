package store

import (
	"context"
	"fmt"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ClaimJob leases the next runnable job to a worker.
func (d *Database) ClaimJob(
	ctx context.Context,
	workerID string,
	leaseDuration time.Duration,
) (domain.Job, error) {
	row, err := d.queries.ClaimJob(ctx, db.ClaimJobParams{
		WorkerID: nullableText(&workerID), LeaseSeconds: leaseDuration.Seconds(),
	})
	if err != nil {
		return domain.Job{}, mapStoreError("claim job", err)
	}
	return jobFromRow(row)
}

// HeartbeatJob extends a job lease owned by a worker.
func (d *Database) HeartbeatJob(
	ctx context.Context,
	jobID uuid.UUID,
	workerID string,
	leaseDuration time.Duration,
) error {
	_, err := d.queries.HeartbeatJob(ctx, db.HeartbeatJobParams{
		LeaseSeconds: leaseDuration.Seconds(), ID: jobID, WorkerID: nullableText(&workerID),
	})
	if err != nil {
		return mapStoreError("heartbeat job", err)
	}
	return nil
}

// CompleteJob marks an owned job successful.
func (d *Database) CompleteJob(ctx context.Context, jobID uuid.UUID, workerID string) error {
	_, err := d.queries.CompleteJob(ctx, db.CompleteJobParams{
		ID: jobID, WorkerID: nullableText(&workerID),
	})
	if err != nil {
		return mapStoreError("complete job", err)
	}
	return nil
}

// RetryJob returns an owned job to the queue after a delay.
func (d *Database) RetryJob(
	ctx context.Context,
	jobID uuid.UUID,
	workerID string,
	delay time.Duration,
	message string,
) error {
	_, err := d.queries.RetryJob(ctx, db.RetryJobParams{
		DelaySeconds: delay.Seconds(), LastError: nullableText(&message),
		ID: jobID, WorkerID: nullableText(&workerID),
	})
	if err != nil {
		return mapStoreError("retry job", err)
	}
	return nil
}

// ExhaustJob atomically fails an owned job and all unfinished run state.
func (d *Database) ExhaustJob(
	ctx context.Context,
	jobID uuid.UUID,
	workerID string,
	message string,
) error {
	_, err := d.queries.ExhaustJob(ctx, db.ExhaustJobParams{
		LastError: nullableText(&message), JobID: jobID, WorkerID: nullableText(&workerID),
	})
	if err != nil {
		return mapStoreError("exhaust job", err)
	}
	return nil
}

// ReapExpiredJobs returns expired leases and their running items to pending.
func (d *Database) ReapExpiredJobs(ctx context.Context) error {
	if _, err := d.queries.ReapExpiredJobs(ctx); err != nil {
		return mapStoreError("reap expired jobs", err)
	}
	return nil
}

// ReleaseWorkerJobs returns a stopping worker's leases and running items to pending.
func (d *Database) ReleaseWorkerJobs(ctx context.Context, workerID string) error {
	if _, err := d.queries.ReleaseWorkerJobs(ctx, nullableText(&workerID)); err != nil {
		return mapStoreError("release worker jobs", err)
	}
	return nil
}

// StartEvalRun moves a pending evaluation run to running.
func (d *Database) StartEvalRun(
	ctx context.Context,
	runID uuid.UUID,
	lease domain.JobLease,
) (domain.EvalRun, error) {
	row, err := d.queries.StartEvalRun(ctx, db.StartEvalRunParams{
		SelectedRunID: runID, JobID: lease.JobID, WorkerID: nullableText(&lease.WorkerID),
	})
	if err != nil {
		return domain.EvalRun{}, mapStoreError("start eval run", err)
	}
	return evalRunFromRow(row)
}

// ListPendingEvalRunItems returns work remaining in a run.
func (d *Database) ListPendingEvalRunItems(
	ctx context.Context,
	runID uuid.UUID,
) ([]domain.EvalRunItem, error) {
	rows, err := d.queries.ListPendingEvalRunItems(ctx, runID)
	if err != nil {
		return nil, mapStoreError("select pending eval run items", err)
	}
	items := make([]domain.EvalRunItem, 0, len(rows))
	for _, row := range rows {
		item, convertErr := pendingEvalRunItemFromRow(row)
		if convertErr != nil {
			return nil, convertErr
		}
		items = append(items, item)
	}
	return items, nil
}

// MarkEvalRunItemRunning starts one pending item.
func (d *Database) MarkEvalRunItemRunning(
	ctx context.Context,
	runID uuid.UUID,
	itemID uuid.UUID,
	lease domain.JobLease,
) error {
	_, err := d.queries.MarkEvalRunItemRunning(ctx, db.MarkEvalRunItemRunningParams{
		SelectedRunID: runID, SelectedItemID: itemID,
		JobID: lease.JobID, WorkerID: nullableText(&lease.WorkerID),
	})
	if err != nil {
		return mapStoreError("start eval run item", err)
	}
	return nil
}

// ResetEvalRunItemPending returns a retryable item to pending.
func (d *Database) ResetEvalRunItemPending(
	ctx context.Context,
	runID uuid.UUID,
	itemID uuid.UUID,
	message string,
	lease domain.JobLease,
) error {
	_, err := d.queries.ResetEvalRunItemPending(ctx, db.ResetEvalRunItemPendingParams{
		SelectedRunID: runID, SelectedItemID: itemID, Error: nullableText(&message),
		JobID: lease.JobID, WorkerID: nullableText(&lease.WorkerID),
	})
	if err != nil {
		return mapStoreError("reset eval run item", err)
	}
	return nil
}

// FailEvalRunItem records a permanent item-level failure.
func (d *Database) FailEvalRunItem(
	ctx context.Context,
	runID uuid.UUID,
	itemID uuid.UUID,
	message string,
	lease domain.JobLease,
) error {
	_, err := d.queries.FailEvalRunItem(ctx, db.FailEvalRunItemParams{
		SelectedRunID: runID, SelectedItemID: itemID, Error: nullableText(&message),
		JobID: lease.JobID, WorkerID: nullableText(&lease.WorkerID),
	})
	if err != nil {
		return mapStoreError("fail eval run item", err)
	}
	return nil
}

// CompleteEvalRunItem atomically replaces scores and marks an item successful.
func (d *Database) CompleteEvalRunItem(
	ctx context.Context,
	runID uuid.UUID,
	itemID uuid.UUID,
	scores []domain.Score,
	lease domain.JobLease,
) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin score transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	if _, err := queries.LockOwnedJob(ctx, db.LockOwnedJobParams{
		ID: lease.JobID, WorkerID: nullableText(&lease.WorkerID),
	}); err != nil {
		return mapStoreError("lock score job", err)
	}
	if err := queries.DeleteEvalRunItemScores(ctx, db.DeleteEvalRunItemScoresParams{
		EvalRunID: runID, DatasetItemID: itemID,
	}); err != nil {
		return mapStoreError("delete stale item scores", err)
	}
	for _, score := range scores {
		if err := insertScore(ctx, queries, score); err != nil {
			return err
		}
	}
	if _, err := queries.CompleteEvalRunItem(ctx, db.CompleteEvalRunItemParams{
		SelectedRunID: runID, SelectedItemID: itemID,
		JobID: lease.JobID, WorkerID: nullableText(&lease.WorkerID),
	}); err != nil {
		return mapStoreError("complete eval run item", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit score transaction: %w", err)
	}
	return nil
}

func jobFromRow(row db.Job) (domain.Job, error) {
	job := domain.Job{
		ID: row.ID, Kind: row.Kind, EvalRunID: row.EvalRunID,
		Status: row.Status, RunAfter: row.RunAfter.Time,
		Attempts: int(row.Attempts), MaxAttempts: int(row.MaxAttempts),
		LockedBy: optionalText(row.LockedBy), LockedAt: optionalTimestamp(row.LockedAt),
		LeaseExpiresAt: optionalTimestamp(row.LeaseExpiresAt), LastError: optionalText(row.LastError),
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	return job, nil
}

func pendingEvalRunItemFromRow(row db.ListPendingEvalRunItemsRow) (domain.EvalRunItem, error) {
	item, err := datasetItemFromRow(db.DatasetItem{
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
		UpdatedAt: row.UpdatedAt.Time, Item: item,
	}, nil
}

func insertScore(ctx context.Context, queries *db.Queries, score domain.Score) error {
	value, err := numeric(score.Value)
	if err != nil {
		return fmt.Errorf("encode score value: %w", err)
	}
	threshold, err := numeric(score.Threshold)
	if err != nil {
		return fmt.Errorf("encode score threshold: %w", err)
	}
	details, err := encodeJSON("score details", score.Details)
	if err != nil {
		return err
	}
	var configID pgtype.UUID
	if score.ScorerConfigID != nil {
		configID = pgtype.UUID{Bytes: *score.ScorerConfigID, Valid: true}
	}
	_, err = queries.InsertOfflineScore(ctx, db.InsertOfflineScoreParams{
		Scorer: score.Scorer, ScorerConfigID: configID, Value: value, Threshold: threshold,
		Passed: score.Passed, Rationale: score.Rationale, Details: details,
		PromptTemplateID: score.PromptTemplateID, JudgeModel: score.JudgeModel,
		JudgeProvider: score.JudgeProvider, JudgeTokens: int32(score.JudgeTokens),
		EvalRunID: score.EvalRunID, DatasetItemID: score.DatasetItemID,
	})
	if err != nil {
		return mapStoreError("insert score", err)
	}
	return nil
}
