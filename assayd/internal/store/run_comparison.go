package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/google/uuid"
)

// CompareEvalRuns returns one paired evidence page and all-page aggregate metadata.
func (d *Database) CompareEvalRuns(
	ctx context.Context,
	query domain.RunComparisonQuery,
) (domain.RunComparisonRead, error) {
	params := db.CompareEvalRunsParams{
		Scorer: query.Scorer, BaselineID: query.BaselineID,
		CandidateID: query.CandidateID, PageSize: int32(query.Limit),
	}
	if query.Cursor != nil {
		params.HasCursor = true
		params.CursorID = *query.Cursor
	}
	rows, err := d.queries.CompareEvalRuns(ctx, params)
	if err != nil {
		return domain.RunComparisonRead{}, mapStoreError("compare eval runs", err)
	}
	if len(rows) == 0 {
		return domain.RunComparisonRead{}, fmt.Errorf(
			"compare eval runs: %w: run pair no longer exists",
			domain.ErrNotFound,
		)
	}
	read := comparisonReadFromRow(rows[0])
	for _, row := range rows {
		if !row.HasItem {
			continue
		}
		item, convertErr := comparisonEvidenceFromRow(row)
		if convertErr != nil {
			return domain.RunComparisonRead{}, convertErr
		}
		read.Items = append(read.Items, item)
	}
	return read, nil
}

func comparisonReadFromRow(row db.CompareEvalRunsRow) domain.RunComparisonRead {
	return domain.RunComparisonRead{
		Items: []domain.RunComparisonEvidence{},
		Aggregate: domain.RunComparisonAggregate{
			SumDelta: row.SumDelta, Matched: int(row.Matched),
			ChangedCases: int(row.ChangedCases), BaselineOnly: int(row.BaselineOnly),
			CandidateOnly: int(row.CandidateOnly), Unscored: int(row.Unscored),
		},
		ContextMismatch:     row.SummaryContextMismatch,
		JudgeConfigMismatch: row.SummaryJudgeConfigMismatch,
	}
}

func comparisonEvidenceFromRow(
	row db.CompareEvalRunsRow,
) (domain.RunComparisonEvidence, error) {
	itemID := optionalUUID(row.DatasetItemID)
	if itemID == nil {
		return domain.RunComparisonEvidence{}, errors.New("comparison dataset item ID is missing")
	}
	baseline, err := comparisonItemFromJSON(row.BaselineItem)
	if err != nil {
		return domain.RunComparisonEvidence{}, fmt.Errorf("decode baseline comparison item: %w", err)
	}
	candidate, err := comparisonItemFromJSON(row.CandidateItem)
	if err != nil {
		return domain.RunComparisonEvidence{}, fmt.Errorf("decode candidate comparison item: %w", err)
	}
	return domain.RunComparisonEvidence{
		DatasetItemID: *itemID, Baseline: baseline, Candidate: candidate,
	}, nil
}

type comparisonItemRecord struct {
	EvalRunID          uuid.UUID              `json:"eval_run_id"`
	DatasetItemID      uuid.UUID              `json:"dataset_item_id"`
	Status             string                 `json:"status"`
	Error              *string                `json:"error"`
	StartedAt          *time.Time             `json:"started_at"`
	FinishedAt         *time.Time             `json:"finished_at"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	GeneratedOutput    *string                `json:"generated_output"`
	GeneratedContext   []domain.Chunk         `json:"generated_context"`
	GeneratedAt        *time.Time             `json:"generated_at"`
	SnapshotDatasetID  uuid.UUID              `json:"snapshot_dataset_id"`
	SnapshotExternalID *string                `json:"snapshot_external_id"`
	SnapshotInput      map[string]any         `json:"snapshot_input"`
	SnapshotOutput     *string                `json:"snapshot_output"`
	SnapshotExpected   *string                `json:"snapshot_expected_output"`
	SnapshotContext    []domain.Chunk         `json:"snapshot_context"`
	SnapshotMetadata   map[string]any         `json:"snapshot_metadata"`
	SnapshotCreatedAt  time.Time              `json:"snapshot_created_at"`
	SnapshotUpdatedAt  time.Time              `json:"snapshot_updated_at"`
	SnapshotOrigin     string                 `json:"snapshot_origin"`
	SelectedScore      *comparisonScoreRecord `json:"selected_score"`
}

type comparisonScoreRecord struct {
	ID               int64          `json:"id"`
	Scorer           string         `json:"scorer"`
	ScorerConfigID   *uuid.UUID     `json:"scorer_config_id"`
	Value            float64        `json:"value"`
	Threshold        float64        `json:"threshold"`
	Passed           bool           `json:"passed"`
	Rationale        string         `json:"rationale"`
	Details          map[string]any `json:"details"`
	PromptTemplateID string         `json:"prompt_template_id"`
	JudgeModel       string         `json:"judge_model"`
	JudgeProvider    string         `json:"judge_provider"`
	JudgeTokens      int            `json:"judge_tokens"`
	EvalRunID        *uuid.UUID     `json:"eval_run_id"`
	DatasetItemID    *uuid.UUID     `json:"dataset_item_id"`
	TraceID          *uuid.UUID     `json:"trace_id"`
	SpanID           *int64         `json:"span_id"`
	SpanStartTime    *time.Time     `json:"span_start_time"`
	JudgedInput      *string        `json:"judged_input"`
	JudgedOutput     *string        `json:"judged_output"`
	JudgedContext    []domain.Chunk `json:"judged_context"`
	JudgedReference  *string        `json:"judged_reference"`
	CreatedAt        time.Time      `json:"created_at"`
}

func comparisonItemFromJSON(data json.RawMessage) (*domain.EvalRunItem, error) {
	if string(data) == "null" {
		return nil, nil
	}
	var record comparisonItemRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	item := &domain.EvalRunItem{
		EvalRunID: record.EvalRunID, DatasetItemID: record.DatasetItemID,
		Status: record.Status, Error: record.Error, StartedAt: record.StartedAt,
		FinishedAt: record.FinishedAt, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		GeneratedOutput: record.GeneratedOutput, GeneratedContext: record.GeneratedContext,
		GeneratedAt: record.GeneratedAt, SnapshotOrigin: record.SnapshotOrigin,
		Item: domain.DatasetItem{
			ID: record.DatasetItemID, DatasetID: record.SnapshotDatasetID,
			ExternalID: record.SnapshotExternalID, Input: record.SnapshotInput,
			Output: record.SnapshotOutput, ExpectedOutput: record.SnapshotExpected,
			Context: record.SnapshotContext, Metadata: record.SnapshotMetadata,
			CreatedAt: record.SnapshotCreatedAt, UpdatedAt: record.SnapshotUpdatedAt,
		},
		Scores: []domain.Score{},
	}
	if record.SelectedScore != nil {
		item.Scores = append(item.Scores, comparisonScore(*record.SelectedScore))
	}
	return item, nil
}

func comparisonScore(record comparisonScoreRecord) domain.Score {
	return domain.Score{
		ID: record.ID, Scorer: record.Scorer, ScorerConfigID: record.ScorerConfigID,
		Value: record.Value, Threshold: record.Threshold, Passed: record.Passed,
		Rationale: record.Rationale, Details: record.Details,
		PromptTemplateID: record.PromptTemplateID, JudgeModel: record.JudgeModel,
		JudgeProvider: record.JudgeProvider, JudgeTokens: record.JudgeTokens,
		EvalRunID: record.EvalRunID, DatasetItemID: record.DatasetItemID,
		TraceID: record.TraceID, SpanID: record.SpanID, SpanStartTime: record.SpanStartTime,
		JudgedInput:   optionalString(record.JudgedInput),
		JudgedOutput:  optionalString(record.JudgedOutput),
		JudgedContext: record.JudgedContext, JudgedReference: record.JudgedReference,
		CreatedAt: record.CreatedAt,
	}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
