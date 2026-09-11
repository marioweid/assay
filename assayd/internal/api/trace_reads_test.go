package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestTraceReadsSupportAdminWithoutWeakeningProjectIsolation(t *testing.T) {
	fixture := newAPIFixture(t)
	primary := fixture.createProjectNamed("Primary", "primary-secret")
	primaryKey := fixture.createAndListKey(primary.ID)
	primaryApplication := fixture.createApplication(primary.ID)
	trace := fixture.ingestTrace(primary.ID, primaryApplication.ID, 1)

	other := fixture.createProjectNamed("Other", "other-secret")
	otherKey := fixture.createAndListKey(other.ID)
	fixture.createApplication(other.ID)

	t.Run("admin lists one application", func(t *testing.T) {
		response := fixture.perform(requestSpec{
			method: http.MethodGet,
			path:   "/v1/traces?application_id=" + primaryApplication.ID,
			token:  adminToken,
		})
		assertStatus(t, response, http.StatusOK)
		var result struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		decodeResponse(t, response, &result)
		if len(result.Items) != 1 || result.Items[0].ID != trace.ID.String() {
			t.Fatalf("admin trace list = %#v, want trace %s", result.Items, trace.ID)
		}
	})

	t.Run("admin list requires application", func(t *testing.T) {
		response := fixture.perform(requestSpec{
			method: http.MethodGet,
			path:   "/v1/traces",
			token:  adminToken,
		})
		assertStatus(t, response, http.StatusBadRequest)
	})

	t.Run("admin gets trace detail", func(t *testing.T) {
		response := fixture.perform(requestSpec{
			method: http.MethodGet,
			path:   "/v1/traces/" + trace.ID.String(),
			token:  adminToken,
		})
		assertStatus(t, response, http.StatusOK)
		var result struct {
			ID    string `json:"id"`
			Spans []any  `json:"spans"`
		}
		decodeResponse(t, response, &result)
		if result.ID != trace.ID.String() || len(result.Spans) != 1 {
			t.Fatalf("admin trace detail = %#v, want trace %s with one span", result, trace.ID)
		}
	})

	t.Run("project keys stay isolated", func(t *testing.T) {
		response := fixture.perform(requestSpec{
			method: http.MethodGet,
			path:   "/v1/traces/" + trace.ID.String(),
			token:  otherKey.Key,
		})
		assertStatus(t, response, http.StatusNotFound)

		response = fixture.perform(requestSpec{
			method: http.MethodGet,
			path:   "/v1/traces?application_id=" + primaryApplication.ID,
			token:  otherKey.Key,
		})
		assertStatus(t, response, http.StatusOK)
		var result struct {
			Items []any `json:"items"`
		}
		decodeResponse(t, response, &result)
		if len(result.Items) != 0 {
			t.Fatalf("cross-project trace list = %#v, want empty", result.Items)
		}

		response = fixture.perform(requestSpec{
			method: http.MethodGet,
			path:   "/v1/traces/" + trace.ID.String(),
			token:  primaryKey.Key,
		})
		assertStatus(t, response, http.StatusOK)
	})
}

func (f *apiFixture) ingestTrace(
	projectID string,
	applicationID string,
	seed byte,
) domain.Trace {
	f.t.Helper()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	trace := domain.Trace{
		ID:            uuid.Must(uuid.NewV7()),
		ApplicationID: uuid.MustParse(applicationID),
		OTelTraceID:   [16]byte{seed},
		RootName:      "answer",
		StartTime:     now,
		EndTime:       now.Add(time.Second),
		Status:        "ok",
		Attributes:    map[string]any{"service.name": "support"},
		Spans: []domain.Span{{
			ApplicationID: uuid.MustParse(applicationID),
			OTelSpanID:    [8]byte{seed},
			Name:          "answer",
			Kind:          "server",
			StartTime:     now,
			EndTime:       now.Add(time.Second),
			DurationMS:    1000,
			StatusCode:    "ok",
			Attributes:    map[string]any{},
			Events:        []domain.SpanEvent{},
		}},
	}
	projectUUID := uuid.MustParse(projectID)
	if err := f.traces.Ingest(f.t.Context(), projectUUID, []domain.Trace{trace}); err != nil {
		f.t.Fatalf("ingest trace: %v", err)
	}
	return trace
}

func TestTraceListValidatesDiscoveryFilters(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProjectNamed("Primary", "primary-secret")
	application := fixture.createApplication(project.ID)
	fixture.ingestTrace(project.ID, application.ID, 1)
	base := "/v1/traces?application_id=" + application.ID
	for _, path := range []string{
		base + "&q=" + strings.Repeat("x", 201),
		base + "&passed=false",
	} {
		response := fixture.perform(requestSpec{method: http.MethodGet, path: path, token: adminToken})
		assertStatus(t, response, http.StatusUnprocessableEntity)
	}
	response := fixture.perform(requestSpec{method: http.MethodGet, path: base, token: adminToken})
	assertStatus(t, response, http.StatusOK)
	var listed struct {
		Items []struct {
			ScoreSummaries []any `json:"score_summaries"`
		} `json:"items"`
	}
	decodeResponse(t, response, &listed)
	if len(listed.Items) != 1 || listed.Items[0].ScoreSummaries == nil {
		t.Fatalf("trace score summaries = %#v, want non-null list", listed.Items)
	}
	response = fixture.perform(requestSpec{
		method: http.MethodGet, path: base + "&q=ANSWER&scorer=groundedness&passed=false",
		token: adminToken,
	})
	assertStatus(t, response, http.StatusOK)
	var result struct {
		Items []struct {
			ScoreSummaries []any `json:"score_summaries"`
		} `json:"items"`
	}
	decodeResponse(t, response, &result)
	if len(result.Items) != 0 {
		t.Fatalf("filtered trace list = %#v, want no unscored traces", result.Items)
	}
}

func TestTraceReadsKeepCyclicSpansVisible(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProjectNamed("Cycle", "cycle-secret")
	application := fixture.createApplication(project.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	firstID := [8]byte{1}
	secondID := [8]byte{2}
	trace := domain.Trace{
		ID:            uuid.Must(uuid.NewV7()),
		ApplicationID: uuid.MustParse(application.ID),
		OTelTraceID:   [16]byte{7},
		RootName:      "cycle",
		StartTime:     now,
		EndTime:       now.Add(time.Second),
		Status:        "ok",
		Attributes:    map[string]any{},
		Spans: []domain.Span{
			spanCycleRow(uuid.MustParse(application.ID), firstID, secondID, "first", now, 1),
			spanCycleRow(uuid.MustParse(application.ID), secondID, firstID, "second", now, 2),
		},
	}
	if err := fixture.traces.Ingest(
		t.Context(), uuid.MustParse(project.ID), []domain.Trace{trace},
	); err != nil {
		t.Fatalf("ingest cyclic trace: %v", err)
	}

	response := fixture.perform(requestSpec{
		method: http.MethodGet, path: "/v1/traces/" + trace.ID.String(), token: adminToken,
	})
	assertStatus(t, response, http.StatusOK)
	var result struct {
		Spans []any `json:"spans"`
	}
	decodeResponse(t, response, &result)
	if collected := countJSONSpans(result.Spans); collected != 2 {
		t.Fatalf("trace detail collected %d spans, want 2", collected)
	}
}

func spanCycleRow(
	applicationID uuid.UUID,
	otelID [8]byte,
	parentID [8]byte,
	name string,
	at time.Time,
	rowID int64,
) domain.Span {
	return domain.Span{
		ID: rowID, ApplicationID: applicationID, OTelSpanID: otelID,
		ParentSpanID: &parentID, Name: name, Kind: "server", StartTime: at,
		EndTime: at.Add(time.Second), DurationMS: 1000, StatusCode: "ok",
		Attributes: map[string]any{}, Events: []domain.SpanEvent{},
	}
}

func countJSONSpans(spans []any) int {
	count := 0
	for _, value := range spans {
		count++
		if node, ok := value.(map[string]any); ok {
			if children, ok := node["children"].([]any); ok {
				count += countJSONSpans(children)
			}
		}
	}
	return count
}
