package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type createEvalRunInput struct {
	Body struct {
		ApplicationID string         `json:"application_id" format:"uuid"`
		DatasetID     string         `json:"dataset_id" format:"uuid"`
		Name          string         `json:"name" minLength:"1"`
		Mode          string         `json:"mode" enum:"score_existing,generate_then_score"`
		Scorers       []string       `json:"scorers" minItems:"1"`
		Params        map[string]any `json:"params,omitempty"`
	}
}

type listEvalRunsInput struct {
	ApplicationID string `query:"application_id" format:"uuid" required:"false"`
	Status        string `query:"status" required:"false"`
	Limit         int    `query:"limit" minimum:"0" maximum:"500" required:"false"`
	Cursor        string `query:"cursor" required:"false"`
}

type evalRunIDInput struct {
	ID string `path:"id" format:"uuid"`
}

type listEvalRunItemsInput struct {
	ID     string `path:"id" format:"uuid"`
	Limit  int    `query:"limit" minimum:"0" maximum:"500" required:"false"`
	Cursor string `query:"cursor" required:"false"`
}

type listEvalRunScoresInput struct {
	ID     string `path:"id" format:"uuid"`
	Limit  int    `query:"limit" minimum:"0" maximum:"500" required:"false"`
	Cursor string `query:"cursor" required:"false"`
}

type evalRunResult struct{ Body evalRunResponse }
type evalRunCollectionResult struct {
	Body struct {
		Items      []evalRunResponse `json:"items"`
		NextCursor string            `json:"next_cursor,omitempty"`
	}
}
type evalRunItemCollectionResult struct {
	Body struct {
		Items      []evalRunItemResponse `json:"items"`
		NextCursor string                `json:"next_cursor,omitempty"`
	}
}
type scoreCollectionResult struct {
	Body struct {
		Items      []scoreResponse `json:"items"`
		NextCursor string          `json:"next_cursor,omitempty"`
	}
}

type evalRunResponse struct {
	ID             string                           `json:"id" format:"uuid"`
	ApplicationID  string                           `json:"application_id" format:"uuid"`
	DatasetID      string                           `json:"dataset_id" format:"uuid"`
	Name           string                           `json:"name"`
	Status         string                           `json:"status"`
	Mode           string                           `json:"mode"`
	Params         map[string]any                   `json:"params"`
	Scorers        []string                         `json:"scorers"`
	Aggregates     map[string]domain.ScoreAggregate `json:"aggregates"`
	TotalItems     int                              `json:"total_items"`
	SucceededItems int                              `json:"succeeded_items"`
	FailedItems    int                              `json:"failed_items"`
	CanceledItems  int                              `json:"canceled_items"`
	StartedAt      *time.Time                       `json:"started_at,omitempty"`
	FinishedAt     *time.Time                       `json:"finished_at,omitempty"`
	Error          *string                          `json:"error,omitempty"`
	CreatedAt      time.Time                        `json:"created_at"`
	UpdatedAt      time.Time                        `json:"updated_at"`
}

type evalRunItemResponse struct {
	EvalRunID        string         `json:"eval_run_id" format:"uuid"`
	DatasetItemID    string         `json:"dataset_item_id" format:"uuid"`
	Status           string         `json:"status"`
	Error            *string        `json:"error,omitempty"`
	StartedAt        *time.Time     `json:"started_at,omitempty"`
	FinishedAt       *time.Time     `json:"finished_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	GeneratedOutput  *string        `json:"generated_output,omitempty"`
	GeneratedContext []domain.Chunk `json:"generated_context,omitempty"`
	GeneratedAt      *time.Time     `json:"generated_at,omitempty"`
}

type scoreResponse struct {
	ID               int64          `json:"id"`
	Scorer           string         `json:"scorer"`
	ScorerConfigID   *string        `json:"scorer_config_id,omitempty" format:"uuid"`
	Value            float64        `json:"value"`
	Threshold        float64        `json:"threshold"`
	Passed           bool           `json:"passed"`
	Rationale        string         `json:"rationale"`
	Details          map[string]any `json:"details"`
	PromptTemplateID string         `json:"prompt_template_id"`
	JudgeModel       string         `json:"judge_model"`
	JudgeProvider    string         `json:"judge_provider"`
	JudgeTokens      int            `json:"judge_tokens"`
	EvalRunID        *string        `json:"eval_run_id,omitempty" format:"uuid"`
	DatasetItemID    *string        `json:"dataset_item_id,omitempty" format:"uuid"`
	TraceID          *string        `json:"trace_id,omitempty" format:"uuid"`
	SpanID           *int64         `json:"span_id,omitempty"`
	SpanStartTime    *time.Time     `json:"span_start_time,omitempty"`
	JudgedInput      *string        `json:"judged_input,omitempty"`
	JudgedOutput     *string        `json:"judged_output,omitempty"`
	JudgedContext    []domain.Chunk `json:"judged_context,omitempty"`
	JudgedReference  *string        `json:"judged_reference,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

type scoreCursorJSON struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
}

func (h *handler) registerEvalRunRoutes() {
	create := h.operation(
		http.MethodPost, "/v1/runs", "create-eval-run", "Create an evaluation run",
		http.StatusNotFound,
	)
	create.DefaultStatus = http.StatusAccepted
	huma.Register(h.api, create, h.createEvalRun)
	huma.Register(h.api, h.operation(
		http.MethodGet, "/v1/runs", "list-eval-runs", "List evaluation runs",
	), h.listEvalRuns)
	huma.Register(h.api, h.operation(
		http.MethodGet, "/v1/runs/{id}", "get-eval-run", "Get an evaluation run",
		http.StatusNotFound,
	), h.getEvalRun)
	huma.Register(h.api, h.operation(
		http.MethodGet, "/v1/runs/{id}/items", "list-eval-run-items",
		"List evaluation run items", http.StatusNotFound,
	), h.listEvalRunItems)
	huma.Register(h.api, h.operation(
		http.MethodGet, "/v1/runs/{id}/scores", "list-eval-run-scores",
		"List evaluation run scores", http.StatusNotFound,
	), h.listEvalRunScores)
	huma.Register(h.api, h.operation(
		http.MethodPost, "/v1/runs/{id}/cancel", "cancel-eval-run",
		"Cancel an evaluation run", http.StatusNotFound, http.StatusConflict,
	), h.cancelEvalRun)
}

func (h *handler) createEvalRun(
	ctx context.Context,
	input *createEvalRunInput,
) (*evalRunResult, error) {
	applicationID, err := parseID(input.Body.ApplicationID, "application ID")
	if err != nil {
		return nil, h.responseError("create eval run", err)
	}
	datasetID, err := parseID(input.Body.DatasetID, "dataset ID")
	if err != nil {
		return nil, h.responseError("create eval run", err)
	}
	run, err := h.evaluations.CreateEvalRun(ctx, domain.CreateEvalRunInput{
		ApplicationID: applicationID, DatasetID: datasetID, Name: input.Body.Name,
		Mode: input.Body.Mode, Scorers: input.Body.Scorers, Params: input.Body.Params,
	})
	if err != nil {
		return nil, h.responseError("create eval run", err)
	}
	return &evalRunResult{Body: evalRunOutput(run)}, nil
}

func (h *handler) listEvalRuns(
	ctx context.Context,
	input *listEvalRunsInput,
) (*evalRunCollectionResult, error) {
	query := domain.EvalRunQuery{PageQuery: domain.PageQuery{Limit: input.Limit}, Status: input.Status}
	var err error
	query.Cursor, err = decodePageCursor(input.Cursor)
	if err == nil && input.ApplicationID != "" {
		id, parseErr := parseID(input.ApplicationID, "application ID")
		query.ApplicationID, err = &id, parseErr
	}
	if err != nil {
		return nil, h.responseError("list eval runs", err)
	}
	page, err := h.evaluations.ListEvalRuns(ctx, query)
	if err != nil {
		return nil, h.responseError("list eval runs", err)
	}
	result := &evalRunCollectionResult{}
	result.Body.Items = make([]evalRunResponse, 0, len(page.Items))
	for _, run := range page.Items {
		result.Body.Items = append(result.Body.Items, evalRunOutput(run))
	}
	result.Body.NextCursor, err = encodePageCursor(page.NextCursor)
	return result, err
}

func (h *handler) getEvalRun(ctx context.Context, input *evalRunIDInput) (*evalRunResult, error) {
	id, err := parseID(input.ID, "eval run ID")
	if err != nil {
		return nil, h.responseError("get eval run", err)
	}
	run, err := h.evaluations.GetEvalRun(ctx, id)
	if err != nil {
		return nil, h.responseError("get eval run", err)
	}
	return &evalRunResult{Body: evalRunOutput(run)}, nil
}

func (h *handler) cancelEvalRun(
	ctx context.Context,
	input *evalRunIDInput,
) (*evalRunResult, error) {
	id, err := parseID(input.ID, "eval run ID")
	if err != nil {
		return nil, h.responseError("cancel eval run", err)
	}
	run, err := h.evaluations.CancelEvalRun(ctx, id)
	if err != nil {
		return nil, h.responseError("cancel eval run", err)
	}
	return &evalRunResult{Body: evalRunOutput(run)}, nil
}

func (h *handler) listEvalRunItems(
	ctx context.Context,
	input *listEvalRunItemsInput,
) (*evalRunItemCollectionResult, error) {
	id, err := parseID(input.ID, "eval run ID")
	if err != nil {
		return nil, h.responseError("list eval run items", err)
	}
	cursor, err := decodePageCursor(input.Cursor)
	if err != nil {
		return nil, h.responseError("list eval run items", err)
	}
	page, err := h.evaluations.ListEvalRunItems(
		ctx, id, domain.PageQuery{Limit: input.Limit, Cursor: cursor},
	)
	if err != nil {
		return nil, h.responseError("list eval run items", err)
	}
	result := &evalRunItemCollectionResult{}
	result.Body.Items = make([]evalRunItemResponse, 0, len(page.Items))
	for _, item := range page.Items {
		result.Body.Items = append(result.Body.Items, evalRunItemOutput(item))
	}
	result.Body.NextCursor, err = encodePageCursor(page.NextCursor)
	return result, err
}

func (h *handler) listEvalRunScores(
	ctx context.Context,
	input *listEvalRunScoresInput,
) (*scoreCollectionResult, error) {
	id, err := parseID(input.ID, "eval run ID")
	if err != nil {
		return nil, h.responseError("list eval run scores", err)
	}
	cursor, err := decodeScoreCursor(input.Cursor)
	if err != nil {
		return nil, h.responseError("list eval run scores", err)
	}
	page, err := h.evaluations.ListEvalRunScores(
		ctx, id, domain.ScoreQuery{Limit: input.Limit, Cursor: cursor},
	)
	if err != nil {
		return nil, h.responseError("list eval run scores", err)
	}
	result := &scoreCollectionResult{}
	result.Body.Items = make([]scoreResponse, 0, len(page.Items))
	for _, score := range page.Items {
		result.Body.Items = append(result.Body.Items, scoreOutput(score))
	}
	result.Body.NextCursor, err = encodeScoreCursor(page.NextCursor)
	return result, err
}

func evalRunOutput(run domain.EvalRun) evalRunResponse {
	return evalRunResponse{
		ID: run.ID.String(), ApplicationID: run.ApplicationID.String(),
		DatasetID: run.DatasetID.String(), Name: run.Name, Status: run.Status,
		Mode: run.Mode, Params: run.Params, Scorers: run.Scorers, Aggregates: run.Aggregates,
		TotalItems: run.TotalItems, SucceededItems: run.SucceededItems,
		FailedItems: run.FailedItems, CanceledItems: run.CanceledItems,
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, Error: run.Error,
		CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
	}
}

func evalRunItemOutput(item domain.EvalRunItem) evalRunItemResponse {
	return evalRunItemResponse{
		EvalRunID: item.EvalRunID.String(), DatasetItemID: item.DatasetItemID.String(),
		Status: item.Status, Error: item.Error, StartedAt: item.StartedAt,
		FinishedAt: item.FinishedAt, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		GeneratedOutput: item.GeneratedOutput, GeneratedContext: item.GeneratedContext,
		GeneratedAt: item.GeneratedAt,
	}
}

func scoreOutput(score domain.Score) scoreResponse {
	response := scoreResponse{
		ID: score.ID, Scorer: score.Scorer, Value: score.Value,
		Threshold: score.Threshold, Passed: score.Passed, Rationale: score.Rationale,
		Details: score.Details, PromptTemplateID: score.PromptTemplateID,
		JudgeModel: score.JudgeModel, JudgeProvider: score.JudgeProvider,
		JudgeTokens: score.JudgeTokens, SpanID: score.SpanID, CreatedAt: score.CreatedAt,
	}
	if score.ScorerConfigID != nil {
		id := score.ScorerConfigID.String()
		response.ScorerConfigID = &id
	}
	if score.EvalRunID != nil {
		id := score.EvalRunID.String()
		response.EvalRunID = &id
	}
	if score.DatasetItemID != nil {
		id := score.DatasetItemID.String()
		response.DatasetItemID = &id
	}
	if score.TraceID != nil {
		id := score.TraceID.String()
		response.TraceID = &id
		response.SpanStartTime = score.SpanStartTime
		response.JudgedInput = &score.JudgedInput
		response.JudgedOutput = &score.JudgedOutput
		response.JudgedContext = score.JudgedContext
		response.JudgedReference = score.JudgedReference
	}
	return response
}

func encodeScoreCursor(cursor *domain.ScoreCursor) (string, error) {
	if cursor == nil {
		return "", nil
	}
	payload, err := json.Marshal(scoreCursorJSON{CreatedAt: cursor.CreatedAt, ID: cursor.ID})
	if err != nil {
		return "", fmt.Errorf("encode score cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeScoreCursor(encoded string) (*domain.ScoreCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("score cursor: %w: invalid encoding", domain.ErrInvalid)
	}
	var cursor scoreCursorJSON
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return nil, fmt.Errorf("score cursor: %w: invalid values", domain.ErrInvalid)
	}
	if cursor.CreatedAt.IsZero() || cursor.ID < 1 {
		return nil, fmt.Errorf("score cursor: %w: invalid values", domain.ErrInvalid)
	}
	return &domain.ScoreCursor{CreatedAt: cursor.CreatedAt, ID: cursor.ID}, nil
}
