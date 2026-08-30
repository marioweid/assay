package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const (
	// ScorerGroundedness verifies generated claims against supplied context.
	ScorerGroundedness = "groundedness"
	// ScorerCorrectness compares generated output with a reference answer.
	ScorerCorrectness = "correctness"

	// GroundednessPromptV1 identifies the first groundedness prompt contract.
	GroundednessPromptV1 = "groundedness@v1"
	// CorrectnessPromptV1 identifies the first correctness prompt contract.
	CorrectnessPromptV1 = "correctness@v1"

	// EvalModeScoreExisting scores outputs already stored in dataset items.
	EvalModeScoreExisting = "score_existing"
	// EvalStatusPending indicates a run or item is queued.
	EvalStatusPending = "pending"
	// EvalStatusRunning indicates a run or item is executing.
	EvalStatusRunning = "running"
	// EvalStatusSucceeded indicates successful completion.
	EvalStatusSucceeded = "succeeded"
	// EvalStatusFailed indicates permanent failure.
	EvalStatusFailed = "failed"
	// EvalStatusCanceled indicates explicit cancellation.
	EvalStatusCanceled = "canceled"
)

// Chunk is one retrieved context unit supplied to a scorer.
type Chunk struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Dataset groups offline evaluation cases for one application.
type Dataset struct {
	ID            uuid.UUID
	ApplicationID uuid.UUID
	Name          string
	Description   *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// DatasetItem is one generated answer and its scoring inputs.
type DatasetItem struct {
	ID             uuid.UUID
	DatasetID      uuid.UUID
	ExternalID     *string
	Input          map[string]any
	Output         *string
	ExpectedOutput *string
	Context        []Chunk
	Metadata       map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CreateDatasetInput contains fields accepted when creating a dataset.
type CreateDatasetInput struct {
	ApplicationID uuid.UUID
	Name          string
	Description   *string
}

// CreateDatasetItemInput contains one offline evaluation case.
type CreateDatasetItemInput struct {
	ExternalID     *string
	Input          map[string]any
	Output         string
	ExpectedOutput *string
	Context        []Chunk
	Metadata       map[string]any
}

// PageCursor identifies a stable position in a UUID-keyed collection.
type PageCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// PageQuery controls UUID-keyed cursor pagination.
type PageQuery struct {
	Limit  int
	Cursor *PageCursor
}

// DatasetQuery filters and paginates datasets.
type DatasetQuery struct {
	PageQuery
	ApplicationID *uuid.UUID
}

// DatasetPage is one page of datasets.
type DatasetPage struct {
	Items      []Dataset
	NextCursor *PageCursor
}

// DatasetItemPage is one page of dataset items.
type DatasetItemPage struct {
	Items      []DatasetItem
	NextCursor *PageCursor
}

// ScorerConfig is a persisted or effective application scorer setting.
type ScorerConfig struct {
	ID               uuid.UUID
	ApplicationID    uuid.UUID
	Scorer           string
	Enabled          bool
	Threshold        float64
	JudgeConfig      *JudgeConfig
	PromptTemplateID string
	Persisted        bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// PutScorerConfigInput contains partial scorer override fields.
type PutScorerConfigInput struct {
	Enabled          *bool
	Threshold        *float64
	JudgeConfig      *JudgeConfigInput
	PromptTemplateID *string
}

// JudgeDefaults supplies process-level judge fallback settings.
type JudgeDefaults struct {
	BaseURL string
	APIKey  string
	Model   string
}

// ResolvedJudgeConfig contains executable judge settings.
type ResolvedJudgeConfig struct {
	BaseURL  string
	APIKey   string
	Model    string
	Provider string
}

// ResolvedScorerConfig contains an effective scorer and judge configuration.
type ResolvedScorerConfig struct {
	ConfigID         *uuid.UUID
	Scorer           string
	Threshold        float64
	PromptTemplateID string
	Judge            ResolvedJudgeConfig
}

// EvalRun is one durable offline scoring execution.
type EvalRun struct {
	ID             uuid.UUID
	ApplicationID  uuid.UUID
	DatasetID      uuid.UUID
	Name           string
	Status         string
	Mode           string
	Params         map[string]any
	Scorers        []string
	Aggregates     map[string]ScoreAggregate
	TotalItems     int
	SucceededItems int
	FailedItems    int
	CanceledItems  int
	StartedAt      *time.Time
	FinishedAt     *time.Time
	Error          *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ScoreAggregate summarizes one scorer across successful items.
type ScoreAggregate struct {
	Mean     float64 `json:"mean"`
	PassRate float64 `json:"pass_rate"`
	N        int     `json:"n"`
}

// CreateEvalRunInput contains a score-existing run request.
type CreateEvalRunInput struct {
	ApplicationID uuid.UUID
	DatasetID     uuid.UUID
	Name          string
	Mode          string
	Scorers       []string
	Params        map[string]any
}

// EvalRunQuery filters and paginates evaluation runs.
type EvalRunQuery struct {
	PageQuery
	ApplicationID *uuid.UUID
	Status        string
}

// EvalRunPage is one page of evaluation runs.
type EvalRunPage struct {
	Items      []EvalRun
	NextCursor *PageCursor
}

// EvalRunItem records one dataset item's outcome within a run.
type EvalRunItem struct {
	EvalRunID     uuid.UUID
	DatasetItemID uuid.UUID
	Status        string
	Error         *string
	StartedAt     *time.Time
	FinishedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Item          DatasetItem
}

// EvalRunItemPage is one page of item outcomes.
type EvalRunItemPage struct {
	Items      []EvalRunItem
	NextCursor *PageCursor
}

// Score is one persisted scorer result for a dataset item.
type Score struct {
	ID               int64
	Scorer           string
	ScorerConfigID   *uuid.UUID
	Value            float64
	Threshold        float64
	Passed           bool
	Rationale        string
	Details          map[string]any
	PromptTemplateID string
	JudgeModel       string
	JudgeProvider    string
	JudgeTokens      int
	EvalRunID        uuid.UUID
	DatasetItemID    uuid.UUID
	CreatedAt        time.Time
}

// ScoreCursor identifies a stable position in a bigint-keyed score collection.
type ScoreCursor struct {
	CreatedAt time.Time
	ID        int64
}

// ScoreQuery controls bigint-keyed score pagination.
type ScoreQuery struct {
	Limit  int
	Cursor *ScoreCursor
}

// ScorePage is one page of score records.
type ScorePage struct {
	Items      []Score
	NextCursor *ScoreCursor
}

// Job is one durable worker lease target.
type Job struct {
	ID             uuid.UUID
	Kind           string
	EvalRunID      uuid.UUID
	Status         string
	RunAfter       time.Time
	Attempts       int
	MaxAttempts    int
	LockedBy       *string
	LockedAt       *time.Time
	LeaseExpiresAt *time.Time
	LastError      *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// JobLease identifies the worker currently authorized to mutate a run.
type JobLease struct {
	JobID    uuid.UUID
	WorkerID string
}

// EvaluationRepository persists the M3 catalog and run intent.
type EvaluationRepository interface {
	GetApplication(context.Context, uuid.UUID) (Application, error)
	GetProject(context.Context, uuid.UUID) (Project, error)
	CreateDataset(context.Context, Dataset) (Dataset, error)
	ListDatasets(context.Context, DatasetQuery) ([]Dataset, error)
	GetDataset(context.Context, uuid.UUID) (Dataset, error)
	DeleteDataset(context.Context, uuid.UUID) error
	CreateDatasetItems(context.Context, uuid.UUID, []DatasetItem) ([]DatasetItem, error)
	ListDatasetItems(context.Context, uuid.UUID, PageQuery) ([]DatasetItem, error)
	CountDatasetItems(context.Context, uuid.UUID) (int, error)
	UpsertScorerConfig(context.Context, ScorerConfig) (ScorerConfig, error)
	ListScorerConfigs(context.Context, uuid.UUID) ([]ScorerConfig, error)
	CreateEvalRun(context.Context, EvalRun, Job) (EvalRun, error)
	ListEvalRuns(context.Context, EvalRunQuery) ([]EvalRun, error)
	GetEvalRun(context.Context, uuid.UUID) (EvalRun, error)
	ListEvalRunItems(context.Context, uuid.UUID, PageQuery) ([]EvalRunItem, error)
	ListEvalRunScores(context.Context, uuid.UUID, ScoreQuery) ([]Score, error)
	CancelEvalRun(context.Context, uuid.UUID) (EvalRun, error)
}
