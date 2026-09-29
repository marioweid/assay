package domain

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Session groups only traces whose real root span explicitly supplied session.id.
type Session struct {
	ID             string
	StartTime      time.Time
	EndTime        time.Time
	TurnCount      int64
	FirstOperation string
	LastTraceID    uuid.UUID
}

// SessionTurn is one root capture, not the repeated history on child generation spans.
type SessionTurn struct {
	ID          uuid.UUID
	RootName    string
	StartTime   time.Time
	EndTime     time.Time
	Status      string
	SpanCount   int
	TotalTokens int64
	Attributes  map[string]any
}

// SessionCursor resumes a session or turn listing after the last returned item.
type SessionCursor struct {
	Time time.Time
	ID   string
}

// SessionQuery scopes a page to an application and optionally one session.
type SessionQuery struct {
	ApplicationID uuid.UUID
	SessionID     string
	Limit         int
	Cursor        *SessionCursor
	Anchor        time.Time
}

// SessionPage contains a bounded page and the cursor needed to continue it.
type SessionPage[T any] struct {
	Items      []T
	NextCursor *SessionCursor
	Anchor     time.Time
}

// SessionRepository reads root-tagged traces within a project boundary.
type SessionRepository interface {
	ListSessions(context.Context, uuid.UUID, SessionQuery) ([]Session, error)
	ListSessionTurns(context.Context, uuid.UUID, SessionQuery) ([]SessionTurn, error)
	ListRecentSessionTurns(context.Context, uuid.UUID, SessionQuery) ([]SessionTurn, error)
}

// SessionService validates and pages scoped session reads.
type SessionService struct{ repository SessionRepository }

// NewSessionService constructs a service over the session repository.
func NewSessionService(repository SessionRepository) *SessionService {
	return &SessionService{repository: repository}
}

func validateSessionQuery(query *SessionQuery) error {
	if query.ApplicationID == uuid.Nil {
		return fmt.Errorf("list sessions: %w: application_id is required", ErrInvalid)
	}
	if query.Limit == 0 {
		query.Limit = defaultTraceLimit
	}
	if query.Limit < 1 || query.Limit > maxTraceLimit {
		return fmt.Errorf("list sessions: %w: limit must be between 1 and 200", ErrInvalid)
	}
	if query.Cursor != nil && (query.Cursor.Time.IsZero() || query.Cursor.ID == "") {
		return fmt.Errorf("list sessions: %w: invalid cursor", ErrInvalid)
	}
	return nil
}

// ValidSessionID accepts a bounded opaque identifier, never an inferred or blank ID.
func ValidSessionID(value string) bool {
	if value == "" || value != strings.Trim(value, " ") || utf8.RuneCountInString(value) > 128 {
		return false
	}
	for _, character := range value {
		if character < 32 || character == 127 {
			return false
		}
	}
	return true
}

// List returns a stable page of sessions scoped to a project and application.
func (s *SessionService) List(
	ctx context.Context, projectID uuid.UUID, query SessionQuery,
) (SessionPage[Session], error) {
	if err := validateSessionQuery(&query); err != nil {
		return SessionPage[Session]{}, err
	}
	if query.Anchor.IsZero() {
		query.Anchor = time.Now().UTC()
	}
	pageSize := query.Limit
	query.Limit++
	items, err := s.repository.ListSessions(ctx, projectID, query)
	if err != nil {
		return SessionPage[Session]{}, fmt.Errorf("list sessions: %w", err)
	}
	page := SessionPage[Session]{Items: items, Anchor: query.Anchor}
	if len(items) > pageSize {
		page.Items = items[:pageSize]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = &SessionCursor{Time: last.EndTime, ID: last.ID}
	}
	return page, nil
}

// RecentTurns returns at most the latest 19 turns in chronological order for model context.
func (s *SessionService) RecentTurns(
	ctx context.Context, projectID uuid.UUID, query SessionQuery,
) ([]SessionTurn, error) {
	if query.Limit == 0 {
		query.Limit = 19
	}
	if err := validateSessionQuery(&query); err != nil {
		return nil, err
	}
	if query.Limit > 19 || query.Cursor != nil || !ValidSessionID(query.SessionID) {
		return nil, fmt.Errorf("list recent session turns: %w: invalid selection", ErrInvalid)
	}
	items, err := s.repository.ListRecentSessionTurns(ctx, projectID, query)
	if err != nil {
		return nil, fmt.Errorf("list recent session turns: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("list recent session turns: %w", ErrNotFound)
	}
	return items, nil
}

// Turns returns a chronological page of root turns from one session.
func (s *SessionService) Turns(
	ctx context.Context, projectID uuid.UUID, query SessionQuery,
) (SessionPage[SessionTurn], error) {
	if err := validateSessionQuery(&query); err != nil {
		return SessionPage[SessionTurn]{}, err
	}
	if err := validateSessionTurnSelection(query); err != nil {
		return SessionPage[SessionTurn]{}, err
	}
	pageSize := query.Limit
	query.Limit++
	items, err := s.repository.ListSessionTurns(ctx, projectID, query)
	if err != nil {
		return SessionPage[SessionTurn]{}, fmt.Errorf("list session turns: %w", err)
	}
	if len(items) == 0 && query.Cursor == nil {
		return SessionPage[SessionTurn]{}, fmt.Errorf("list session turns: %w", ErrNotFound)
	}
	page := SessionPage[SessionTurn]{Items: items}
	if len(items) > pageSize {
		page.Items = items[:pageSize]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = &SessionCursor{Time: last.StartTime, ID: last.ID.String()}
	}
	return page, nil
}

func validateSessionTurnSelection(query SessionQuery) error {
	if !ValidSessionID(query.SessionID) {
		return fmt.Errorf("list session turns: %w: invalid session ID", ErrInvalid)
	}
	if query.Cursor != nil {
		if _, err := uuid.Parse(query.Cursor.ID); err != nil {
			return fmt.Errorf("list session turns: %w: invalid cursor", ErrInvalid)
		}
	}
	return nil
}
