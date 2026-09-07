package api_test

import (
	"net/http"
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
