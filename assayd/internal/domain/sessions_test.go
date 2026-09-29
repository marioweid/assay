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

func TestSessionPagesAndValidation(t *testing.T) {
	app := uuid.New()
	project := uuid.New()
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	last := uuid.New()
	repository := &fakeSessionRepository{
		sessions: []Session{{ID: "second", EndTime: at}, {ID: "first", EndTime: at.Add(-time.Hour)}},
		turns:    []SessionTurn{{ID: uuid.New(), StartTime: at}, {ID: last, StartTime: at}},
	}
	service := NewSessionService(repository)
	query := SessionQuery{ApplicationID: app, SessionID: "second", Limit: 1, Anchor: at.Add(time.Hour)}
	page, err := service.List(t.Context(), project, query)
	if err != nil || len(page.Items) != 1 || page.NextCursor == nil ||
		page.NextCursor.ID != "second" || repository.query.Limit != 2 || repository.project != project {
		t.Fatalf("session page = %#v, query = %#v, error = %v", page, repository.query, err)
	}
	turns, err := service.Turns(t.Context(), project, query)
	if err != nil || len(turns.Items) != 1 || turns.NextCursor == nil ||
		turns.NextCursor.ID != repository.turns[0].ID.String() {
		t.Fatalf("turn page = %#v, error = %v", turns, err)
	}
	for _, invalid := range []string{"", " ", " a", "a\n", "a\x00", strings.Repeat("a", 129)} {
		query.SessionID = invalid
		if _, err := service.Turns(t.Context(), project, query); !errors.Is(err, ErrInvalid) {
			t.Errorf("session ID %q: %v, want invalid", invalid, err)
		}
	}
	if !ValidSessionID("\u00a0session") || !ValidSessionID("..") || !ValidSessionID("a%2Fb") {
		t.Fatal("opaque IDs allowed by the SQL projection must remain addressable")
	}
	query.SessionID = "second"
	recent, err := service.RecentTurns(t.Context(), project, SessionQuery{
		ApplicationID: app, SessionID: "second",
	})
	if err != nil || len(recent) != 2 || repository.query.Limit != 19 {
		t.Fatalf("recent turns = %#v, query = %#v, error = %v", recent, repository.query, err)
	}
	if _, err := service.RecentTurns(t.Context(), project, SessionQuery{
		ApplicationID: app, SessionID: "second", Limit: 20,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbounded recent turns: %v", err)
	}
	query.Cursor = &SessionCursor{Time: at, ID: "bad-turn-id"}
	if _, err := service.Turns(t.Context(), project, query); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid turn cursor: %v", err)
	}
	repository.turns = nil
	query.Cursor = nil
	if _, err := service.Turns(t.Context(), project, query); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing session: %v", err)
	}
}
