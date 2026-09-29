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
	"github.com/google/uuid"
)

type listSessionsInput struct {
	Authorization string `header:"Authorization" required:"false"`
	XAPIKey       string `header:"x-api-key" required:"false"`
	ApplicationID string `query:"application_id" format:"uuid" required:"true"`
	Limit         int    `query:"limit" minimum:"0" maximum:"200" required:"false"`
	Cursor        string `query:"cursor" required:"false"`
}

type listSessionTurnsInput struct {
	Authorization string `header:"Authorization" required:"false"`
	XAPIKey       string `header:"x-api-key" required:"false"`
	ApplicationID string `query:"application_id" format:"uuid" required:"true"`
	Limit         int    `query:"limit" minimum:"0" maximum:"200" required:"false"`
	Cursor        string `query:"cursor" required:"false"`
	SessionID     string `query:"session_id" maxLength:"128" required:"true"`
}

type recentSessionTurnsInput struct {
	Authorization string `header:"Authorization" required:"false"`
	XAPIKey       string `header:"x-api-key" required:"false"`
	ApplicationID string `query:"application_id" format:"uuid" required:"true"`
	SessionID     string `query:"session_id" maxLength:"128" required:"true"`
	Limit         int    `query:"limit" minimum:"0" maximum:"19" required:"false"`
}

type sessionResponse struct {
	ID             string    `json:"id"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	TurnCount      int64     `json:"turn_count"`
	FirstOperation string    `json:"first_operation"`
	LastTraceID    string    `json:"last_trace_id" format:"uuid"`
}

type sessionTurnResponse struct {
	ID          string         `json:"id" format:"uuid"`
	RootName    string         `json:"root_name"`
	StartTime   time.Time      `json:"start_time"`
	EndTime     time.Time      `json:"end_time"`
	Status      string         `json:"status"`
	SpanCount   int            `json:"span_count"`
	TotalTokens int64          `json:"total_tokens"`
	Attributes  map[string]any `json:"attributes"`
}

type sessionCollectionResult struct {
	Body struct {
		Items      []sessionResponse `json:"items" nullable:"false"`
		NextCursor string            `json:"next_cursor,omitempty"`
	}
}

type sessionTurnsResult struct {
	Body struct {
		Items      []sessionTurnResponse `json:"items" nullable:"false"`
		NextCursor string                `json:"next_cursor,omitempty"`
	}
}

type sessionCursorJSON struct {
	Time   time.Time `json:"time"`
	ID     string    `json:"id"`
	Anchor time.Time `json:"anchor,omitempty"`
}

func (h *handler) registerSessionRoutes() {
	huma.Register(h.api, traceReadOperation(h.projectOperation(
		http.MethodGet, "/v1/sessions", "list-sessions", "List tagged sessions",
		http.StatusBadRequest,
	)), h.listSessions)
	huma.Register(h.api, traceReadOperation(h.projectOperation(
		http.MethodGet, "/v1/session-turns", "list-session-turns", "List session turns",
		http.StatusBadRequest, http.StatusNotFound,
	)), h.listSessionTurns)
	huma.Register(h.api, traceReadOperation(h.projectOperation(
		http.MethodGet, "/v1/session-turns/recent", "list-recent-session-turns",
		"List recent session turns", http.StatusBadRequest, http.StatusNotFound,
	)), h.listRecentSessionTurns)
}

func (h *handler) sessionQuery(
	ctx context.Context, input *listSessionsInput, sessionID string, turns bool,
) (uuid.UUID, domain.SessionQuery, error) {
	applicationID, err := parseID(input.ApplicationID, "application ID")
	if err != nil {
		return uuid.Nil, domain.SessionQuery{}, err
	}
	projectID, err := h.sessionProject(ctx, input, applicationID)
	if err != nil {
		return uuid.Nil, domain.SessionQuery{}, err
	}
	query := domain.SessionQuery{
		ApplicationID: applicationID, SessionID: sessionID, Limit: input.Limit,
	}
	if input.Cursor != "" {
		query.Cursor, query.Anchor, err = sessionQueryCursor(input.Cursor, turns)
		if err != nil {
			return uuid.Nil, domain.SessionQuery{}, err
		}
	}
	return projectID, query, nil
}

func (h *handler) sessionProject(
	ctx context.Context, input *listSessionsInput, applicationID uuid.UUID,
) (uuid.UUID, error) {
	if !h.isAdmin(input.Authorization, input.XAPIKey) {
		return h.authenticateProject(ctx, input.Authorization, input.XAPIKey)
	}
	application, err := h.service.GetApplication(ctx, applicationID)
	if err != nil {
		return uuid.Nil, err
	}
	return application.ProjectID, nil
}

func sessionQueryCursor(value string, turns bool) (*domain.SessionCursor, time.Time, error) {
	encoded, err := decodeSessionCursor(value)
	if err != nil {
		return nil, time.Time{}, err
	}
	if turns && !encoded.Anchor.IsZero() || !turns && encoded.Anchor.IsZero() {
		return nil, time.Time{}, fmt.Errorf("session cursor: %w: wrong page type", domain.ErrInvalid)
	}
	return &domain.SessionCursor{Time: encoded.Time, ID: encoded.ID}, encoded.Anchor, nil
}

func (h *handler) listSessions(
	ctx context.Context, input *listSessionsInput,
) (*sessionCollectionResult, error) {
	projectID, query, err := h.sessionQuery(ctx, input, "", false)
	if err != nil {
		return nil, h.responseError("list sessions", err)
	}
	page, err := h.sessions.List(ctx, projectID, query)
	if err != nil {
		return nil, h.responseError("list sessions", err)
	}
	result := &sessionCollectionResult{}
	result.Body.Items = make([]sessionResponse, 0, len(page.Items))
	for _, item := range page.Items {
		result.Body.Items = append(result.Body.Items, sessionResponse{
			ID: item.ID, StartTime: item.StartTime, EndTime: item.EndTime,
			TurnCount: item.TurnCount, FirstOperation: item.FirstOperation,
			LastTraceID: item.LastTraceID.String(),
		})
	}
	result.Body.NextCursor, err = encodeSessionCursor(page.NextCursor, page.Anchor)
	if err != nil {
		return nil, h.responseError("list sessions", err)
	}
	return result, nil
}

func (h *handler) listSessionTurns(
	ctx context.Context, input *listSessionTurnsInput,
) (*sessionTurnsResult, error) {
	projectID, query, err := h.sessionQuery(ctx, &listSessionsInput{
		Authorization: input.Authorization, XAPIKey: input.XAPIKey,
		ApplicationID: input.ApplicationID, Limit: input.Limit, Cursor: input.Cursor,
	}, input.SessionID, true)
	if err != nil {
		return nil, h.responseError("list session turns", err)
	}
	page, err := h.sessions.Turns(ctx, projectID, query)
	if err != nil {
		return nil, h.responseError("list session turns", err)
	}
	result := &sessionTurnsResult{}
	result.Body.Items = sessionTurnsOutput(page.Items)
	result.Body.NextCursor, err = encodeSessionCursor(page.NextCursor, time.Time{})
	if err != nil {
		return nil, h.responseError("list session turns", err)
	}
	return result, nil
}

func (h *handler) listRecentSessionTurns(
	ctx context.Context, input *recentSessionTurnsInput,
) (*sessionTurnsResult, error) {
	projectID, query, err := h.sessionQuery(ctx, &listSessionsInput{
		Authorization: input.Authorization, XAPIKey: input.XAPIKey,
		ApplicationID: input.ApplicationID, Limit: input.Limit,
	}, input.SessionID, true)
	if err != nil {
		return nil, h.responseError("list recent session turns", err)
	}
	items, err := h.sessions.RecentTurns(ctx, projectID, query)
	if err != nil {
		return nil, h.responseError("list recent session turns", err)
	}
	result := &sessionTurnsResult{}
	result.Body.Items = sessionTurnsOutput(items)
	return result, nil
}

func sessionTurnsOutput(items []domain.SessionTurn) []sessionTurnResponse {
	output := make([]sessionTurnResponse, 0, len(items))
	for _, item := range items {
		output = append(output, sessionTurnResponse{
			ID: item.ID.String(), RootName: item.RootName, StartTime: item.StartTime,
			EndTime: item.EndTime, Status: item.Status, SpanCount: item.SpanCount,
			TotalTokens: item.TotalTokens, Attributes: item.Attributes,
		})
	}
	return output
}

func encodeSessionCursor(cursor *domain.SessionCursor, anchor time.Time) (string, error) {
	if cursor == nil {
		return "", nil
	}
	payload, err := json.Marshal(sessionCursorJSON{Time: cursor.Time, ID: cursor.ID, Anchor: anchor})
	if err != nil {
		return "", fmt.Errorf("encode session cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeSessionCursor(value string) (sessionCursorJSON, error) {
	if len(value) > 4096 {
		return sessionCursorJSON{}, fmt.Errorf("session cursor: %w: too long", domain.ErrInvalid)
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return sessionCursorJSON{}, fmt.Errorf("session cursor: %w: invalid encoding", domain.ErrInvalid)
	}
	var cursor sessionCursorJSON
	if json.Unmarshal(payload, &cursor) != nil || cursor.Time.IsZero() ||
		!domain.ValidSessionID(cursor.ID) && uuid.Validate(cursor.ID) != nil {
		return sessionCursorJSON{}, fmt.Errorf("session cursor: %w: invalid values", domain.ErrInvalid)
	}
	return cursor, nil
}
