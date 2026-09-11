package api

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type listTracesInput struct {
	Authorization string    `header:"Authorization" required:"false"`
	XAPIKey       string    `header:"x-api-key" required:"false"`
	ApplicationID string    `query:"application_id" format:"uuid" required:"false"`
	Start         time.Time `query:"start" required:"false"`
	End           time.Time `query:"end" required:"false"`
	Status        string    `query:"status" required:"false"`
	//nolint:lll // Huma requires the public parameter documentation in one struct tag.
	Q string `query:"q" required:"false" doc:"Trimmed literal case-insensitive root-name search, or exact Assay UUID or 32-hex OpenTelemetry trace ID; maximum 200 characters after trimming."`
	//nolint:lll // Huma requires the public parameter documentation in one struct tag.
	Scorer string `query:"scorer" required:"false" enum:"groundedness,correctness" doc:"Filter by the latest online score for this scorer."`
	//nolint:lll // Huma requires the public parameter documentation in one struct tag.
	Passed string `query:"passed" required:"false" enum:"true,false" doc:"Filter score pass state; requires scorer."`
	Limit  int    `query:"limit" minimum:"0" maximum:"200" required:"false"`
	Cursor string `query:"cursor" required:"false"`
}

type traceIDInput struct {
	Authorization string `header:"Authorization" required:"false"`
	XAPIKey       string `header:"x-api-key" required:"false"`
	ID            string `path:"id" format:"uuid"`
}

type scoreTracesInput struct {
	Authorization string `header:"Authorization" required:"false"`
	XAPIKey       string `header:"x-api-key" required:"false"`
	Body          struct {
		TraceIDs []string `json:"trace_ids" minItems:"1"`
		Scorers  []string `json:"scorers" minItems:"1"`
	}
}

type scoringEligibilityInput struct {
	ID string `path:"id" format:"uuid"`
}

type attachTraceReferenceInput struct {
	Authorization string `header:"Authorization" required:"false"`
	XAPIKey       string `header:"x-api-key" required:"false"`
	ID            string `path:"id" format:"uuid"`
	Body          struct {
		ReferenceAnswer string `json:"reference_answer" minLength:"1"`
	}
}

type scoringEligibilityReasonResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type scoringEligibilityResponse struct {
	Scorer   string                             `json:"scorer"`
	Eligible bool                               `json:"eligible"`
	Reasons  []scoringEligibilityReasonResponse `json:"reasons" nullable:"false"`
}

type scoringEligibilityResult struct {
	Body struct {
		Items []scoringEligibilityResponse `json:"items" nullable:"false"`
	}
}

type traceCollectionResult struct {
	Body struct {
		Items      []traceListResponse `json:"items"`
		NextCursor string              `json:"next_cursor,omitempty"`
	}
}

type traceResult struct {
	Body traceResponse
}

type scoringTaskCollectionResult struct {
	Body struct {
		Items []scoringTaskResponse `json:"items"`
	}
}

type scoringTaskResponse struct {
	ID      string  `json:"id" format:"uuid"`
	TraceID string  `json:"trace_id" format:"uuid"`
	Scorer  string  `json:"scorer"`
	Status  string  `json:"status"`
	Error   *string `json:"error,omitempty"`
}

type traceScoreSummaryResponse struct {
	Scorer    string    `json:"scorer"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Passed    bool      `json:"passed"`
	CreatedAt time.Time `json:"created_at"`
}

type traceResponse struct {
	ID              string                `json:"id" format:"uuid"`
	ApplicationID   string                `json:"application_id" format:"uuid"`
	OTelTraceID     string                `json:"otel_trace_id"`
	RootName        string                `json:"root_name"`
	StartTime       time.Time             `json:"start_time"`
	EndTime         time.Time             `json:"end_time"`
	Status          string                `json:"status"`
	SpanCount       int                   `json:"span_count"`
	TotalTokens     int64                 `json:"total_tokens"`
	TotalCost       *string               `json:"total_cost,omitempty"`
	ReferenceAnswer *string               `json:"reference_answer,omitempty"`
	Attributes      map[string]any        `json:"attributes"`
	Spans           []*spanResponse       `json:"spans,omitempty"`
	Scores          []scoreResponse       `json:"scores,omitempty"`
	ScoringTasks    []scoringTaskResponse `json:"scoring_tasks,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

type traceListResponse struct {
	ID              string                      `json:"id" format:"uuid"`
	ApplicationID   string                      `json:"application_id" format:"uuid"`
	OTelTraceID     string                      `json:"otel_trace_id"`
	RootName        string                      `json:"root_name"`
	StartTime       time.Time                   `json:"start_time"`
	EndTime         time.Time                   `json:"end_time"`
	Status          string                      `json:"status"`
	SpanCount       int                         `json:"span_count"`
	TotalTokens     int64                       `json:"total_tokens"`
	TotalCost       *string                     `json:"total_cost,omitempty"`
	ReferenceAnswer *string                     `json:"reference_answer,omitempty"`
	Attributes      map[string]any              `json:"attributes"`
	ScoreSummaries  []traceScoreSummaryResponse `json:"score_summaries" nullable:"false"`
	CreatedAt       time.Time                   `json:"created_at"`
	UpdatedAt       time.Time                   `json:"updated_at"`
}

type spanResponse struct {
	ID              int64              `json:"id"`
	OTelSpanID      string             `json:"otel_span_id"`
	ParentSpanID    *string            `json:"parent_span_id,omitempty"`
	Name            string             `json:"name"`
	Kind            string             `json:"kind"`
	OperationName   string             `json:"operation_name,omitempty"`
	StartTime       time.Time          `json:"start_time"`
	EndTime         time.Time          `json:"end_time"`
	DurationMS      int64              `json:"duration_ms"`
	StatusCode      string             `json:"status_code"`
	StatusMessage   string             `json:"status_message,omitempty"`
	IsScorable      bool               `json:"is_scorable"`
	ScorableKind    string             `json:"scorable_kind,omitempty"`
	Attributes      map[string]any     `json:"attributes"`
	Events          []domain.SpanEvent `json:"events"`
	InputTokens     int64              `json:"input_tokens"`
	OutputTokens    int64              `json:"output_tokens"`
	ReferenceAnswer *string            `json:"reference_answer,omitempty"`
	Children        []*spanResponse    `json:"children"`
}

type traceCursorJSON struct {
	StartTime time.Time `json:"start_time"`
	ID        string    `json:"id"`
}

func (h *handler) registerTraceRoutes() {
	score := traceReadOperation(h.projectOperation(
		http.MethodPost, "/v1/traces/score", "score-traces", "Queue trace scoring",
		http.StatusNotFound,
	))
	score.DefaultStatus = http.StatusAccepted
	huma.Register(h.api, score, h.scoreTraces)
	huma.Register(h.api, h.operation(
		http.MethodGet, "/v1/traces/{id}/scoring-eligibility", "get-trace-scoring-eligibility",
		"Get trace scoring eligibility", http.StatusNotFound,
	), h.scoringEligibility)
	huma.Register(h.api, traceReadOperation(h.projectOperation(
		http.MethodPatch, "/v1/traces/{id}/reference", "attach-trace-reference",
		"Attach a trace reference", http.StatusNotFound,
	)), h.attachTraceReference)
	huma.Register(h.api, traceReadOperation(h.projectOperation(
		http.MethodGet,
		"/v1/traces",
		"list-traces",
		"List traces",
		http.StatusBadRequest,
	)), h.listTraces)
	huma.Register(h.api, traceReadOperation(h.projectOperation(
		http.MethodGet,
		"/v1/traces/{id}",
		"get-trace",
		"Get a trace and its span tree",
		http.StatusNotFound,
	)), h.getTrace)
	remove := h.operation(
		http.MethodDelete, "/v1/traces/{id}", "delete-trace", "Delete a trace",
		http.StatusNotFound,
	)
	remove.DefaultStatus = http.StatusNoContent
	huma.Register(h.api, remove, h.deleteTrace)
}

func (h *handler) scoringEligibility(
	ctx context.Context,
	input *scoringEligibilityInput,
) (*scoringEligibilityResult, error) {
	traceID, err := parseID(input.ID, "trace ID")
	if err != nil {
		return nil, h.responseError("get trace scoring eligibility", err)
	}
	items, err := h.traces.ScoringEligibility(ctx, traceID)
	if err != nil {
		return nil, h.responseError("get trace scoring eligibility", err)
	}
	result := &scoringEligibilityResult{}
	result.Body.Items = make([]scoringEligibilityResponse, 0, len(items))
	for _, item := range items {
		output := scoringEligibilityResponse{
			Scorer: item.Scorer, Eligible: item.Eligible,
			Reasons: make([]scoringEligibilityReasonResponse, 0, len(item.Reasons)),
		}
		for _, reason := range item.Reasons {
			output.Reasons = append(output.Reasons, scoringEligibilityReasonResponse{
				Code: reason.Code, Message: reason.Message,
			})
		}
		result.Body.Items = append(result.Body.Items, output)
	}
	return result, nil
}

func (h *handler) scoreTraces(
	ctx context.Context,
	input *scoreTracesInput,
) (*scoringTaskCollectionResult, error) {
	traceIDs := make([]uuid.UUID, 0, len(input.Body.TraceIDs))
	for _, value := range input.Body.TraceIDs {
		traceID, parseErr := parseID(value, "trace ID")
		if parseErr != nil {
			return nil, h.responseError("score traces", parseErr)
		}
		traceIDs = append(traceIDs, traceID)
	}
	jobs, err := h.queueTraceScores(ctx, input, traceIDs)
	if err != nil {
		return nil, h.responseError("score traces", err)
	}
	result := &scoringTaskCollectionResult{}
	result.Body.Items = make([]scoringTaskResponse, 0, len(jobs))
	for _, job := range jobs {
		result.Body.Items = append(result.Body.Items, scoringTaskOutput(job))
	}
	return result, nil
}

func (h *handler) queueTraceScores(
	ctx context.Context,
	input *scoreTracesInput,
	traceIDs []uuid.UUID,
) ([]domain.Job, error) {
	if h.isAdmin(input.Authorization) {
		return h.traces.QueueScoresAdmin(ctx, traceIDs, input.Body.Scorers)
	}
	projectID, err := h.authenticateProject(ctx, input.Authorization, input.XAPIKey)
	if err != nil {
		return nil, err
	}
	return h.traces.QueueScores(ctx, projectID, traceIDs, input.Body.Scorers, true)
}

func (h *handler) attachTraceReference(
	ctx context.Context,
	input *attachTraceReferenceInput,
) (*traceResult, error) {
	traceID, err := parseID(input.ID, "trace ID")
	if err != nil {
		return nil, h.responseError("attach trace reference", err)
	}
	trace, err := h.attachReference(ctx, input, traceID)
	if err != nil {
		return nil, h.responseError("attach trace reference", err)
	}
	return &traceResult{Body: traceOutput(trace, true)}, nil
}

func (h *handler) attachReference(
	ctx context.Context,
	input *attachTraceReferenceInput,
	traceID uuid.UUID,
) (domain.Trace, error) {
	if h.isAdmin(input.Authorization) {
		return h.traces.AttachReferenceAdmin(ctx, traceID, input.Body.ReferenceAnswer)
	}
	projectID, err := h.authenticateProject(ctx, input.Authorization, input.XAPIKey)
	if err != nil {
		return domain.Trace{}, err
	}
	return h.traces.AttachReference(ctx, projectID, traceID, input.Body.ReferenceAnswer)
}

func (h *handler) deleteTrace(
	ctx context.Context,
	input *traceIDInput,
) (*emptyOutput, error) {
	traceID, err := parseID(input.ID, "trace ID")
	if err != nil {
		return nil, h.responseError("delete trace", err)
	}
	if err := h.traces.DeleteAdmin(ctx, traceID); err != nil {
		return nil, h.responseError("delete trace", err)
	}
	return &emptyOutput{}, nil
}

func (h *handler) listTraces(
	ctx context.Context,
	input *listTracesInput,
) (*traceCollectionResult, error) {
	query, err := traceQuery(input)
	if err != nil {
		return nil, h.responseError("list traces", err)
	}
	projectID, err := h.traceListProjectID(ctx, input, query)
	if err != nil {
		return nil, err
	}
	page, err := h.traces.List(ctx, projectID, query)
	if err != nil {
		return nil, h.responseError("list traces", err)
	}
	result := &traceCollectionResult{}
	result.Body.Items = make([]traceListResponse, 0, len(page.Items))
	for _, trace := range page.Items {
		result.Body.Items = append(result.Body.Items, traceListOutput(trace))
	}
	result.Body.NextCursor, err = encodeTraceCursor(page.NextCursor)
	if err != nil {
		return nil, h.responseError("list traces", err)
	}
	return result, nil
}

func (h *handler) traceListProjectID(
	ctx context.Context,
	input *listTracesInput,
	query domain.TraceQuery,
) (uuid.UUID, error) {
	if !h.isAdmin(input.Authorization) {
		projectID, err := h.authenticateProject(ctx, input.Authorization, input.XAPIKey)
		if err != nil {
			return uuid.Nil, h.responseError("list traces", err)
		}
		return projectID, nil
	}
	if query.ApplicationID == nil {
		return uuid.Nil, huma.Error400BadRequest(
			"application_id is required for admin trace lists",
		)
	}
	application, err := h.service.GetApplication(ctx, *query.ApplicationID)
	if err != nil {
		return uuid.Nil, h.responseError("list traces", err)
	}
	return application.ProjectID, nil
}

func (h *handler) getTrace(
	ctx context.Context,
	input *traceIDInput,
) (*traceResult, error) {
	admin := h.isAdmin(input.Authorization)
	var projectID uuid.UUID
	var err error
	if !admin {
		projectID, err = h.authenticateProject(ctx, input.Authorization, input.XAPIKey)
		if err != nil {
			return nil, h.responseError("get trace", err)
		}
	}
	traceID, err := parseID(input.ID, "trace ID")
	if err != nil {
		return nil, h.responseError("get trace", err)
	}
	var trace domain.Trace
	if admin {
		trace, err = h.traces.GetAdmin(ctx, traceID)
	} else {
		trace, err = h.traces.Get(ctx, projectID, traceID)
	}
	if err != nil {
		return nil, h.responseError("get trace", err)
	}
	return &traceResult{Body: traceOutput(trace, true)}, nil
}

func traceQuery(input *listTracesInput) (domain.TraceQuery, error) {
	query := domain.TraceQuery{
		Status: input.Status,
		Q:      input.Q,
		Scorer: input.Scorer,
		Limit:  input.Limit,
	}
	if input.Passed != "" {
		passed := input.Passed == "true"
		query.Passed = &passed
	}
	if !input.Start.IsZero() {
		query.Start = &input.Start
	}
	if !input.End.IsZero() {
		query.End = &input.End
	}
	var err error
	if input.ApplicationID != "" {
		applicationID, parseErr := parseID(input.ApplicationID, "application ID")
		if parseErr != nil {
			return domain.TraceQuery{}, parseErr
		}
		query.ApplicationID = &applicationID
	}
	query.Cursor, err = decodeTraceCursor(input.Cursor)
	if err != nil {
		return domain.TraceQuery{}, err
	}
	return query, nil
}

func encodeTraceCursor(cursor *domain.TraceCursor) (string, error) {
	if cursor == nil {
		return "", nil
	}
	payload, err := json.Marshal(traceCursorJSON{
		StartTime: cursor.StartTime,
		ID:        cursor.ID.String(),
	})
	if err != nil {
		return "", fmt.Errorf("encode trace cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeTraceCursor(encoded string) (*domain.TraceCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("trace cursor: %w: invalid encoding", domain.ErrInvalid)
	}
	var cursor traceCursorJSON
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return nil, fmt.Errorf("trace cursor: %w: invalid JSON", domain.ErrInvalid)
	}
	id, err := uuid.Parse(cursor.ID)
	if err != nil || cursor.StartTime.IsZero() {
		return nil, fmt.Errorf("trace cursor: %w: invalid values", domain.ErrInvalid)
	}
	return &domain.TraceCursor{StartTime: cursor.StartTime, ID: id}, nil
}

func traceOutput(trace domain.Trace, includeSpans bool) traceResponse {
	response := traceResponse{
		ID:              trace.ID.String(),
		ApplicationID:   trace.ApplicationID.String(),
		OTelTraceID:     hex.EncodeToString(trace.OTelTraceID[:]),
		RootName:        trace.RootName,
		StartTime:       trace.StartTime,
		EndTime:         trace.EndTime,
		Status:          trace.Status,
		SpanCount:       trace.SpanCount,
		TotalTokens:     trace.TotalTokens,
		TotalCost:       trace.TotalCost,
		ReferenceAnswer: trace.ReferenceAnswer,
		Attributes:      trace.Attributes,
		CreatedAt:       trace.CreatedAt,
		UpdatedAt:       trace.UpdatedAt,
	}
	if includeSpans {
		response.Spans = spanTree(trace.Spans)
		response.Scores = make([]scoreResponse, 0, len(trace.Scores))
		for _, score := range trace.Scores {
			response.Scores = append(response.Scores, scoreOutput(score))
		}
		response.ScoringTasks = make([]scoringTaskResponse, 0, len(trace.ScoringTasks))
		for _, job := range trace.ScoringTasks {
			response.ScoringTasks = append(response.ScoringTasks, scoringTaskOutput(job))
		}
	}
	return response
}

func traceListOutput(trace domain.Trace) traceListResponse {
	response := traceListResponse{
		ID:              trace.ID.String(),
		ApplicationID:   trace.ApplicationID.String(),
		OTelTraceID:     hex.EncodeToString(trace.OTelTraceID[:]),
		RootName:        trace.RootName,
		StartTime:       trace.StartTime,
		EndTime:         trace.EndTime,
		Status:          trace.Status,
		SpanCount:       trace.SpanCount,
		TotalTokens:     trace.TotalTokens,
		TotalCost:       trace.TotalCost,
		ReferenceAnswer: trace.ReferenceAnswer,
		Attributes:      trace.Attributes,
		ScoreSummaries:  make([]traceScoreSummaryResponse, 0, len(trace.ScoreSummaries)),
		CreatedAt:       trace.CreatedAt,
		UpdatedAt:       trace.UpdatedAt,
	}
	for _, summary := range trace.ScoreSummaries {
		response.ScoreSummaries = append(response.ScoreSummaries, traceScoreSummaryResponse{
			Scorer: summary.Scorer, Value: summary.Value, Threshold: summary.Threshold,
			Passed: summary.Passed, CreatedAt: summary.CreatedAt,
		})
	}
	return response
}

func scoringTaskOutput(job domain.Job) scoringTaskResponse {
	response := scoringTaskResponse{
		ID: job.ID.String(), Scorer: job.Scorer, Status: job.Status, Error: job.LastError,
	}
	if job.TraceID != nil {
		response.TraceID = job.TraceID.String()
	}
	return response
}

func spanOutput(span domain.Span) *spanResponse {
	response := &spanResponse{
		ID:              span.ID,
		OTelSpanID:      hex.EncodeToString(span.OTelSpanID[:]),
		Name:            span.Name,
		Kind:            span.Kind,
		OperationName:   span.OperationName,
		StartTime:       span.StartTime,
		EndTime:         span.EndTime,
		DurationMS:      span.DurationMS,
		StatusCode:      span.StatusCode,
		StatusMessage:   span.StatusMessage,
		IsScorable:      span.IsScorable,
		ScorableKind:    span.ScorableKind,
		Attributes:      span.Attributes,
		Events:          span.Events,
		InputTokens:     span.InputTokens,
		OutputTokens:    span.OutputTokens,
		ReferenceAnswer: span.ReferenceAnswer,
		Children:        make([]*spanResponse, 0),
	}
	if span.ParentSpanID != nil {
		parent := hex.EncodeToString(span.ParentSpanID[:])
		response.ParentSpanID = &parent
	}
	return response
}
