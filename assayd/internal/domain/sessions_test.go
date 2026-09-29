package domain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeSessionRepository struct {
	sessions []Session
	turns    []SessionTurn
	query    SessionQuery
	project  uuid.UUID
}

func (f *fakeSessionRepository) ListSessions(
	_ context.Context, project uuid.UUID, query SessionQuery,
) ([]Session, error) {
	f.project, f.query = project, query
	return f.sessions, nil
}

func (f *fakeSessionRepository) ListSessionTurns(
	_ context.Context, project uuid.UUID, query SessionQuery,
) ([]SessionTurn, error) {
	f.project, f.query = project, query
	return f.turns, nil
}

func (f *fakeSessionRepository) ListRecentSessionTurns(
	_ context.Context, project uuid.UUID, query SessionQuery,
) ([]SessionTurn, error) {
	f.project, f.query = project, query
	return f.turns, nil
}

func sessionTestFixture() (*SessionService, *fakeSessionRepository, uuid.UUID, SessionQuery) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	repository := &fakeSessionRepository{
		sessions: []Session{{ID: "second", EndTime: at}, {ID: "first", EndTime: at.Add(-time.Hour)}},
		turns:    []SessionTurn{{ID: uuid.New(), StartTime: at}, {ID: uuid.New(), StartTime: at}},
	}
	query := SessionQuery{
		ApplicationID: uuid.New(), SessionID: "second", Limit: 1, Anchor: at.Add(time.Hour),
	}
	return NewSessionService(repository), repository, uuid.New(), query
}

func TestSessionListPage(t *testing.T) {
	service, repository, project, query := sessionTestFixture()
	page, err := service.List(t.Context(), project, query)
	if err != nil || len(page.Items) != 1 || page.NextCursor == nil ||
		page.NextCursor.ID != "second" || repository.query.Limit != 2 || repository.project != project {
		t.Fatalf("session page = %#v, query = %#v, error = %v", page, repository.query, err)
	}
}

func TestSessionTurnPage(t *testing.T) {
	service, repository, project, query := sessionTestFixture()
	turns, err := service.Turns(t.Context(), project, query)
	if err != nil || len(turns.Items) != 1 || turns.NextCursor == nil ||
		turns.NextCursor.ID != repository.turns[0].ID.String() {
		t.Fatalf("turn page = %#v, error = %v", turns, err)
	}
}

func TestSessionIDValidation(t *testing.T) {
	service, _, project, query := sessionTestFixture()
	for _, invalid := range []string{"", " ", " a", "a\n", "a\x00", strings.Repeat("a", 129)} {
		query.SessionID = invalid
		if _, err := service.Turns(t.Context(), project, query); !errors.Is(err, ErrInvalid) {
			t.Errorf("session ID %q: %v, want invalid", invalid, err)
		}
	}
	for _, valid := range []string{"\u00a0session", "..", "a%2Fb"} {
		if !ValidSessionID(valid) {
			t.Errorf("opaque session ID %q must remain addressable", valid)
		}
	}
}

func TestSessionRecentTurnLimit(t *testing.T) {
	service, repository, project, query := sessionTestFixture()
	recent, err := service.RecentTurns(t.Context(), project, SessionQuery{
		ApplicationID: query.ApplicationID, SessionID: query.SessionID,
	})
	if err != nil || len(recent) != 2 || repository.query.Limit != 19 {
		t.Fatalf("recent turns = %#v, query = %#v, error = %v", recent, repository.query, err)
	}
	query.Limit = 20
	if _, err := service.RecentTurns(t.Context(), project, query); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbounded recent turns: %v", err)
	}
}

func TestSessionTurnsRejectInvalidCursorAndMissingSession(t *testing.T) {
	service, repository, project, query := sessionTestFixture()
	query.Cursor = &SessionCursor{Time: repository.turns[0].StartTime, ID: "bad-turn-id"}
	if _, err := service.Turns(t.Context(), project, query); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid turn cursor: %v", err)
	}
	repository.turns = nil
	query.Cursor = nil
	if _, err := service.Turns(t.Context(), project, query); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing session: %v", err)
	}
}
