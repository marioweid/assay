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
	Limit         int       `query:"limit" minimum:"0" maximum:"200" required:"false"`
	Cursor        string    `query:"cursor" required:"false"`
}

type traceIDInput struct {
	Authorization string `header:"Authorization" required:"false"`
	XAPIKey       string `header:"x-api-key" required:"false"`
	ID            string `path:"id" format:"uuid"`
}

type traceCollectionResult struct {
	Body struct {
		Items      []traceResponse `json:"items"`
		NextCursor string          `json:"next_cursor,omitempty"`
	}
}

type traceResult struct {
	Body traceResponse
}

type traceResponse struct {
	ID              string          `json:"id" format:"uuid"`
	ApplicationID   string          `json:"application_id" format:"uuid"`
	OTelTraceID     string          `json:"otel_trace_id"`
	RootName        string          `json:"root_name"`
	StartTime       time.Time       `json:"start_time"`
	EndTime         time.Time       `json:"end_time"`
	Status          string          `json:"status"`
	SpanCount       int             `json:"span_count"`
	TotalTokens     int64           `json:"total_tokens"`
	TotalCost       *string         `json:"total_cost,omitempty"`
	ReferenceAnswer *string         `json:"reference_answer,omitempty"`
	Attributes      map[string]any  `json:"attributes"`
	Spans           []*spanResponse `json:"spans,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
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
	huma.Register(h.api, h.projectOperation(
		http.MethodGet,
		"/v1/traces",
		"list-traces",
		"List traces",
	), h.listTraces)
	huma.Register(h.api, h.projectOperation(
		http.MethodGet,
		"/v1/traces/{id}",
		"get-trace",
		"Get a trace and its span tree",
		http.StatusNotFound,
	), h.getTrace)
}

func (h *handler) listTraces(
	ctx context.Context,
	input *listTracesInput,
) (*traceCollectionResult, error) {
	projectID, err := h.authenticateProject(ctx, input.Authorization, input.XAPIKey)
	if err != nil {
		return nil, h.responseError("list traces", err)
	}
	query, err := traceQuery(input)
	if err != nil {
		return nil, h.responseError("list traces", err)
	}
	page, err := h.traces.List(ctx, projectID, query)
	if err != nil {
		return nil, h.responseError("list traces", err)
	}
	result := &traceCollectionResult{}
	result.Body.Items = make([]traceResponse, 0, len(page.Items))
	for _, trace := range page.Items {
		result.Body.Items = append(result.Body.Items, traceOutput(trace, false))
	}
	result.Body.NextCursor, err = encodeTraceCursor(page.NextCursor)
	if err != nil {
		return nil, h.responseError("list traces", err)
	}
	return result, nil
}

func (h *handler) getTrace(
	ctx context.Context,
	input *traceIDInput,
) (*traceResult, error) {
	projectID, err := h.authenticateProject(ctx, input.Authorization, input.XAPIKey)
	if err != nil {
		return nil, h.responseError("get trace", err)
	}
	traceID, err := parseID(input.ID, "trace ID")
	if err != nil {
		return nil, h.responseError("get trace", err)
	}
	trace, err := h.traces.Get(ctx, projectID, traceID)
	if err != nil {
		return nil, h.responseError("get trace", err)
	}
	return &traceResult{Body: traceOutput(trace, true)}, nil
}

func traceQuery(input *listTracesInput) (domain.TraceQuery, error) {
	query := domain.TraceQuery{
		Status: input.Status,
		Limit:  input.Limit,
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
	}
	return response
}

func spanTree(spans []domain.Span) []*spanResponse {
	nodes := make(map[string]*spanResponse, len(spans))
	ordered := make([]*spanResponse, 0, len(spans))
	for _, span := range spans {
		node := spanOutput(span)
		nodes[node.OTelSpanID] = node
		ordered = append(ordered, node)
	}
	roots := make([]*spanResponse, 0)
	for _, node := range ordered {
		if node.ParentSpanID != nil {
			if parent, found := nodes[*node.ParentSpanID]; found && parent != node {
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		roots = append(roots, node)
	}
	return roots
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
