package store_test

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	secretcrypto "github.com/marioweid/assay/assayd/internal/crypto"
	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/migrate"
	"github.com/marioweid/assay/assayd/internal/store"
	"github.com/marioweid/assay/assayd/internal/testutil"

	"github.com/google/uuid"
)

func TestTraceStoreUpsertsIncrementalExportsAndScopesReads(t *testing.T) {
	database := openTraceDatabase(t)
	service, traceService := newTraceServices(t, database)
	project, application := createTraceApplication(t, service, "Primary")
	otherProject, otherApplication := createTraceApplication(t, service, "Other")
	trace := traceFixture(application.ID)
	ingestTraceFixtures(t, traceService, project.ID, trace)
	assertStoredTrace(t, traceService, project.ID, otherProject.ID)
	assertSpanOwnership(t, traceService, project.ID, application.ID, otherApplication.ID)
}

func assertSpanOwnership(
	t *testing.T,
	service *domain.TraceService,
	projectID uuid.UUID,
	applicationID uuid.UUID,
	otherApplicationID uuid.UUID,
) {
	t.Helper()
	trace := traceFixture(applicationID)
	trace.ID = uuid.Must(uuid.NewV7())
	trace.OTelTraceID = [16]byte{9}
	trace.Spans = trace.Spans[:1]
	trace.Spans[0].ApplicationID = otherApplicationID
	if err := service.Ingest(t.Context(), projectID, []domain.Trace{trace}); err == nil {
		t.Fatal("ingest span owned by another application succeeded")
	}
}

func newTraceServices(
	t *testing.T,
	database *store.Database,
) (*domain.Service, *domain.TraceService) {
	t.Helper()
	cipher, err := secretcrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	service := domain.NewService(database, cipher)
	return service, domain.NewTraceService(database, service, 3)
}

func traceFixture(applicationID uuid.UUID) domain.Trace {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	traceID := [16]byte{1, 2, 3}
	rootID := [8]byte{1}
	childID := [8]byte{2}
	return domain.Trace{
		ID:            uuid.Must(uuid.NewV7()),
		ApplicationID: applicationID,
		OTelTraceID:   traceID,
		RootName:      "answer",
		StartTime:     now,
		EndTime:       now.Add(2 * time.Second),
		Status:        "ok",
		Attributes:    map[string]any{"service.name": "support"},
		Spans: []domain.Span{
			{
				ApplicationID: applicationID,
				OTelSpanID:    rootID,
				Name:          "answer",
				StartTime:     now,
				EndTime:       now.Add(2 * time.Second),
				StatusCode:    "ok",
				Attributes:    map[string]any{"root": true},
				Events:        []domain.SpanEvent{},
			},
			{
				ApplicationID: applicationID,
				OTelSpanID:    childID,
				ParentSpanID:  &rootID,
				Name:          "generation",
				StartTime:     now.Add(time.Second),
				EndTime:       now.Add(2 * time.Second),
				StatusCode:    "ok",
				InputTokens:   10,
				OutputTokens:  5,
				Attributes:    map[string]any{},
				Events:        []domain.SpanEvent{},
			},
		},
	}
}

func ingestTraceFixtures(
	t *testing.T,
	service *domain.TraceService,
	projectID uuid.UUID,
	trace domain.Trace,
) {
	t.Helper()
	if err := service.Ingest(t.Context(), projectID, []domain.Trace{trace, trace}); err != nil {
		t.Fatalf("ingest duplicate trace export: %v", err)
	}
	rootID := trace.Spans[0].OTelSpanID
	now := trace.StartTime
	incremental := trace
	incremental.ID = uuid.Must(uuid.NewV7())
	incremental.Spans = []domain.Span{{
		ApplicationID: trace.ApplicationID,
		OTelSpanID:    [8]byte{3},
		ParentSpanID:  &rootID,
		Name:          "tool",
		StartTime:     now.Add(1500 * time.Millisecond),
		EndTime:       now.Add(1700 * time.Millisecond),
		StatusCode:    "ok",
		Attributes:    map[string]any{},
		Events:        []domain.SpanEvent{},
	}}
	if err := service.Ingest(t.Context(), projectID, []domain.Trace{incremental}); err != nil {
		t.Fatalf("ingest incremental trace export: %v", err)
	}
}

func assertStoredTrace(
	t *testing.T,
	service *domain.TraceService,
	projectID uuid.UUID,
	otherProjectID uuid.UUID,
) {
	t.Helper()
	page, err := service.List(t.Context(), projectID, domain.TraceQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list traces: %v", err)
	}
	assertTracePage(t, page)
	detail, err := service.Get(t.Context(), projectID, page.Items[0].ID)
	if err != nil {
		t.Fatalf("get trace detail: %v", err)
	}
	assertTraceDetail(t, detail)
	if _, err := service.Get(t.Context(), otherProjectID, detail.ID); !errors.Is(
		err,
		domain.ErrNotFound,
	) {
		t.Fatalf("cross-project trace error = %v, want not found", err)
	}
}

func assertTracePage(t *testing.T, page domain.TracePage) {
	t.Helper()
	if len(page.Items) != 1 {
		t.Fatalf("trace summary = %#v", page.Items)
	}
	if page.Items[0].SpanCount != 3 || page.Items[0].TotalTokens != 15 {
		t.Fatalf("trace summary = %#v", page.Items[0])
	}
}

func assertTraceDetail(t *testing.T, detail domain.Trace) {
	t.Helper()
	if len(detail.Spans) != 3 {
		t.Fatalf("trace detail = %#v", detail)
	}
	if detail.RootName != "answer" {
		t.Fatalf("trace root = %q, want answer", detail.RootName)
	}
}

func openTraceDatabase(t *testing.T) *store.Database {
	t.Helper()
	database, err := store.Open(t.Context(), testutil.Postgres(t))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrate.Up(t.Context(), database.MigrationDB(), logger); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return database
}

func createTraceApplication(
	t *testing.T,
	service *domain.Service,
	name string,
) (domain.Project, domain.Application) {
	t.Helper()
	project, err := service.CreateProject(t.Context(), domain.CreateProjectInput{Name: name})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	application, err := service.CreateApplication(t.Context(), domain.CreateApplicationInput{
		ProjectID: project.ID,
		Name:      name,
		Slug:      name,
	})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	return project, application
}
