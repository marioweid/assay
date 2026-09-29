package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestSessionsRequireRootAndScopeProjectAndApplication(t *testing.T) {
	fixture := newScopedSessionFixture(t)
	assertRootSessionMembership(t, fixture)
	assertRecentSessionTurns(t, fixture)
	assertSessionTurnCapture(t, fixture)
	assertSessionProjectAndAppScope(t, fixture)
	assertTurnCursor(t, fixture)
	assertTurnDeletion(t, fixture)
}

type scopedSessionFixture struct {
	sessions       *domain.SessionService
	traces         *domain.TraceService
	projectID      uuid.UUID
	otherProjectID uuid.UUID
	otherAppID     uuid.UUID
	firstID        uuid.UUID
	secondID       uuid.UUID
	query          domain.SessionQuery
}

func newScopedSessionFixture(t *testing.T) scopedSessionFixture {
	t.Helper()
	database := openTraceDatabase(t)
	service, traces := newTraceServices(t, database)
	project, application := createTraceApplication(t, service, "Sessions")
	_, otherApp := createApplicationInProject(t, service, project.ID, "Second")
	otherProject, foreignApp := createTraceApplication(t, service, "Foreign")
	first := taggedTrace(application.ID, 1, "shared", time.Second)
	second := taggedTrace(application.ID, 2, "shared", 2*time.Second)
	untagged := taggedTrace(application.ID, 3, "", 3*time.Second)
	untagged.Spans[1].Attributes["session.id"] = "shared"
	for _, trace := range []struct {
		project uuid.UUID
		value   domain.Trace
	}{
		{project.ID, first}, {project.ID, second}, {project.ID, untagged},
		{project.ID, taggedTrace(otherApp.ID, 4, "shared", time.Second)},
		{otherProject.ID, taggedTrace(foreignApp.ID, 5, "shared", time.Second)},
	} {
		if err := traces.Ingest(t.Context(), trace.project, []domain.Trace{trace.value}); err != nil {
			t.Fatalf("ingest trace: %v", err)
		}
	}
	return scopedSessionFixture{
		sessions: domain.NewSessionService(database), traces: traces,
		projectID: project.ID, otherProjectID: otherProject.ID, otherAppID: otherApp.ID,
		firstID: first.ID, secondID: second.ID,
		query: domain.SessionQuery{
			ApplicationID: application.ID, SessionID: "shared",
			Anchor: time.Now().Add(time.Hour),
		},
	}
}

func assertRootSessionMembership(t *testing.T, fixture scopedSessionFixture) {
	t.Helper()
	query := fixture.query
	query.SessionID = ""
	page, err := fixture.sessions.List(t.Context(), fixture.projectID, query)
	if err != nil || len(page.Items) != 1 || page.Items[0].TurnCount != 2 {
		t.Fatalf("session list = %#v, %v", page, err)
	}
}

func assertRecentSessionTurns(t *testing.T, fixture scopedSessionFixture) {
	t.Helper()
	recent, err := fixture.sessions.RecentTurns(t.Context(), fixture.projectID, fixture.query)
	if err != nil || len(recent) != 2 ||
		recent[0].ID != fixture.firstID || recent[1].ID != fixture.secondID {
		t.Fatalf("recent chronological turns = %#v, %v", recent, err)
	}
}

func assertSessionTurnCapture(t *testing.T, fixture scopedSessionFixture) {
	t.Helper()
	turns, err := fixture.sessions.Turns(t.Context(), fixture.projectID, fixture.query)
	if err != nil || len(turns.Items) != 2 ||
		turns.Items[0].ID != fixture.firstID || turns.Items[1].ID != fixture.secondID {
		t.Fatalf("session turns = %#v, %v", turns, err)
	}
	if _, found := turns.Items[0].Attributes["gen_ai.input.messages"]; !found {
		t.Fatalf("root capture absent: %#v", turns.Items[0])
	}
}

func assertSessionProjectAndAppScope(t *testing.T, fixture scopedSessionFixture) {
	t.Helper()
	if _, err := fixture.sessions.Turns(
		t.Context(), fixture.otherProjectID, fixture.query,
	); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-project session: %v", err)
	}
	query := fixture.query
	query.ApplicationID = fixture.otherAppID
	other, err := fixture.sessions.Turns(t.Context(), fixture.projectID, query)
	if err != nil || len(other.Items) != 1 {
		t.Fatalf("cross-application isolation = %#v, %v", other, err)
	}
}

func assertTurnCursor(t *testing.T, fixture scopedSessionFixture) {
	t.Helper()
	query := fixture.query
	query.Limit = 1
	firstPage, err := fixture.sessions.Turns(t.Context(), fixture.projectID, query)
	if err != nil || firstPage.NextCursor == nil || len(firstPage.Items) != 1 {
		t.Fatalf("first page = %#v, %v", firstPage, err)
	}
	query.Cursor = firstPage.NextCursor
	secondPage, err := fixture.sessions.Turns(t.Context(), fixture.projectID, query)
	if err != nil || len(secondPage.Items) != 1 || secondPage.Items[0].ID != fixture.secondID {
		t.Fatalf("second page = %#v, %v", secondPage, err)
	}
}

func assertTurnDeletion(t *testing.T, fixture scopedSessionFixture) {
	t.Helper()
	if err := fixture.traces.DeleteAdmin(t.Context(), fixture.firstID); err != nil {
		t.Fatalf("delete turn: %v", err)
	}
	turns, err := fixture.sessions.Turns(t.Context(), fixture.projectID, fixture.query)
	if err != nil || len(turns.Items) != 1 || turns.Items[0].ID != fixture.secondID {
		t.Fatalf("turn after delete = %#v, %v", turns, err)
	}
}

func TestSessionListCursorOrdersTiedActivity(t *testing.T) {
	database := openTraceDatabase(t)
	service, traces := newTraceServices(t, database)
	project, application := createTraceApplication(t, service, "Tied sessions")
	sessions := domain.NewSessionService(database)
	for index, sessionID := range []string{"alpha", "beta", "gamma"} {
		trace := taggedTrace(application.ID, byte(index+1), sessionID, time.Second)
		if err := traces.Ingest(t.Context(), project.ID, []domain.Trace{trace}); err != nil {
			t.Fatalf("ingest %s session: %v", sessionID, err)
		}
	}
	query := domain.SessionQuery{
		ApplicationID: application.ID, Anchor: time.Now().Add(time.Hour), Limit: 1,
	}
	for _, want := range []string{"gamma", "beta", "alpha"} {
		page, err := sessions.List(t.Context(), project.ID, query)
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != want {
			t.Fatalf("session page for %q = %#v, %v", want, page, err)
		}
		if (page.NextCursor != nil) != (want != "alpha") {
			t.Fatalf("next cursor for %q = %#v", want, page.NextCursor)
		}
		query.Cursor = page.NextCursor
	}
}

func TestSessionAppearsOnlyAfterRootArrives(t *testing.T) {
	database := openTraceDatabase(t)
	service, traces := newTraceServices(t, database)
	project, application := createTraceApplication(t, service, "Late root")
	sessions := domain.NewSessionService(database)
	trace := taggedTrace(application.ID, 1, "late", time.Second)
	child := trace.Spans[1]
	child.Attributes["session.id"] = "late"
	partial := trace
	partial.Spans = []domain.Span{child}
	if err := traces.Ingest(t.Context(), project.ID, []domain.Trace{partial}); err != nil {
		t.Fatalf("ingest child first: %v", err)
	}
	query := domain.SessionQuery{ApplicationID: application.ID, Anchor: time.Now().Add(time.Hour)}
	before, err := sessions.List(t.Context(), project.ID, query)
	if err != nil || len(before.Items) != 0 {
		t.Fatalf("child-only membership = %#v, %v", before, err)
	}
	trace.Spans = trace.Spans[:1]
	if err := traces.Ingest(t.Context(), project.ID, []domain.Trace{trace}); err != nil {
		t.Fatalf("ingest root: %v", err)
	}
	after, err := sessions.List(t.Context(), project.ID, query)
	if err != nil || len(after.Items) != 1 || after.Items[0].ID != "late" {
		t.Fatalf("late root membership = %#v, %v", after, err)
	}
}

func createApplicationInProject(
	t *testing.T, service *domain.Service, projectID uuid.UUID, name string,
) (uuid.UUID, domain.Application) {
	t.Helper()
	application, err := service.CreateApplication(t.Context(), domain.CreateApplicationInput{
		ProjectID: projectID, Name: name, Slug: name,
	})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	return projectID, application
}

func taggedTrace(
	applicationID uuid.UUID, marker byte, sessionID string, offset time.Duration,
) domain.Trace {
	trace := traceFixture(applicationID)
	trace.ID = uuid.Must(uuid.NewV7())
	trace.OTelTraceID = [16]byte{marker}
	trace.StartTime = trace.StartTime.Add(offset)
	trace.EndTime = trace.EndTime.Add(offset)
	for index := range trace.Spans {
		trace.Spans[index].StartTime = trace.Spans[index].StartTime.Add(offset)
		trace.Spans[index].EndTime = trace.Spans[index].EndTime.Add(offset)
	}
	trace.Spans[0].Attributes["gen_ai.input.messages"] = `[ {"role":"user","content":"hello"} ]`
	if sessionID != "" {
		trace.Spans[0].Attributes["session.id"] = sessionID
	}
	return trace
}
