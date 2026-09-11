package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestAdminTraceMutationsRemainProjectIsolated(t *testing.T) {
	fixture := newAPIFixture(t)
	primary := fixture.createProjectNamed("Primary", "primary-secret")
	primaryKey := fixture.createAndListKey(primary.ID)
	primaryApplication := fixture.createApplication(primary.ID)
	trace := fixture.ingestScorableTrace(primary.ID, primaryApplication.ID, 1)

	other := fixture.createProjectNamed("Other", "other-secret")
	otherKey := fixture.createAndListKey(other.ID)
	fixture.createApplication(other.ID)

	t.Run("admin queues and references one trace", func(t *testing.T) {
		response := fixture.perform(requestSpec{
			method: http.MethodPost, path: "/v1/traces/score", token: adminToken,
			body: `{"trace_ids":["` + trace.ID.String() + `"],"scorers":["groundedness"]}`,
		})
		assertStatus(t, response, http.StatusAccepted)

		response = fixture.perform(requestSpec{
			method: http.MethodPatch, path: "/v1/traces/" + trace.ID.String() + "/reference",
			token: adminToken, body: `{"reference_answer":"expected answer"}`,
		})
		assertStatus(t, response, http.StatusOK)
	})

	t.Run("foreign project key stays scoped", func(t *testing.T) {
		for _, spec := range []requestSpec{
			{
				method: http.MethodPost, path: "/v1/traces/score", token: otherKey.Key,
				body: `{"trace_ids":["` + trace.ID.String() + `"],"scorers":["groundedness"]}`,
			},
			{
				method: http.MethodPatch, path: "/v1/traces/" + trace.ID.String() + "/reference",
				token: otherKey.Key, body: `{"reference_answer":"intrusion"}`,
			},
		} {
			assertStatus(t, fixture.perform(spec), http.StatusNotFound)
		}
		response := fixture.perform(requestSpec{
			method: http.MethodPost, path: "/v1/traces/score", token: primaryKey.Key,
			body: `{"trace_ids":["` + trace.ID.String() + `"],"scorers":["groundedness"]}`,
		})
		assertStatus(t, response, http.StatusAccepted)
	})

	t.Run("blank admin reference is rejected", func(t *testing.T) {
		response := fixture.perform(requestSpec{
			method: http.MethodPatch, path: "/v1/traces/" + trace.ID.String() + "/reference",
			token: adminToken, body: `{"reference_answer":"   "}`,
		})
		assertStatus(t, response, http.StatusUnprocessableEntity)
	})
}

func TestTraceScoringEligibilityIsAdminOnlyAndMatchesQueueing(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProjectNamed("Primary", "primary-secret")
	application := fixture.createApplication(project.ID)
	trace := fixture.ingestScorableTrace(project.ID, application.ID, 1)
	path := "/v1/traces/" + trace.ID.String() + "/scoring-eligibility"

	assertStatus(
		t, fixture.perform(requestSpec{method: http.MethodGet, path: path}), http.StatusUnauthorized,
	)
	response := fixture.perform(requestSpec{method: http.MethodGet, path: path, token: adminToken})
	assertStatus(t, response, http.StatusOK)
	var result struct {
		Items []struct {
			Scorer   string `json:"scorer"`
			Eligible bool   `json:"eligible"`
			Reasons  []struct {
				Code string `json:"code"`
			} `json:"reasons"`
		} `json:"items"`
	}
	decodeResponse(t, response, &result)
	if len(result.Items) != 2 || result.Items[0].Scorer != domain.ScorerGroundedness ||
		!result.Items[0].Eligible || result.Items[1].Scorer != domain.ScorerCorrectness ||
		result.Items[1].Eligible || len(result.Items[1].Reasons) != 1 ||
		result.Items[1].Reasons[0].Code != "missing_reference" {
		t.Fatalf("eligibility = %#v", result)
	}
	response = fixture.perform(requestSpec{
		method: http.MethodPost, path: "/v1/traces/score", token: adminToken,
		body: `{"trace_ids":["` + trace.ID.String() + `"],"scorers":["correctness"]}`,
	})
	assertStatus(t, response, http.StatusUnprocessableEntity)
}

func TestAdminMixedProjectScoreBatchIsAtomic(t *testing.T) {
	fixture := newAPIFixture(t)
	primary := fixture.createProjectNamed("Primary", "primary-secret")
	primaryApplication := fixture.createApplication(primary.ID)
	primaryTrace := fixture.ingestScorableTrace(primary.ID, primaryApplication.ID, 1)
	other := fixture.createProjectNamed("Other", "other-secret")
	otherApplication := fixture.createApplication(other.ID)
	otherTrace := fixture.ingestScorableTrace(other.ID, otherApplication.ID, 2)

	response := fixture.perform(requestSpec{
		method: http.MethodPost, path: "/v1/traces/score", token: adminToken,
		body: `{"trace_ids":["` + primaryTrace.ID.String() + `","` +
			otherTrace.ID.String() + `"],"scorers":["groundedness"]}`,
	})
	assertStatus(t, response, http.StatusUnprocessableEntity)

	if count := fixture.countTraceJobs(t, primaryTrace.ID); count != 0 {
		t.Fatalf("mixed batch created %d jobs", count)
	}
}

func TestAdminDeletesTracesAndTerminalRuns(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProjectNamed("Primary", "primary-secret")
	application := fixture.createApplication(project.ID)
	trace := fixture.ingestScorableTrace(project.ID, application.ID, 1)

	t.Run("delete trace removes evidence", func(t *testing.T) {
		response := fixture.perform(requestSpec{
			method: http.MethodDelete, path: "/v1/traces/" + trace.ID.String(), token: adminToken,
		})
		assertStatus(t, response, http.StatusNoContent)
		response = fixture.perform(requestSpec{
			method: http.MethodDelete, path: "/v1/traces/" + trace.ID.String(), token: adminToken,
		})
		assertStatus(t, response, http.StatusNotFound)
	})

	t.Run("delete trace requires admin", func(t *testing.T) {
		response := fixture.perform(requestSpec{
			method: http.MethodDelete, path: "/v1/traces/" + uuid.Must(uuid.NewV7()).String(),
			token: adminToken,
		})
		assertStatus(t, response, http.StatusNotFound)
	})

	dataset := fixture.createDatasetForMutation(application.ID, "eval-cases")
	item := fixture.createDatasetItemForMutation(dataset, "case-1")
	runID := fixture.createRunForMutation(application.ID, dataset)

	t.Run("active run deletion conflicts", func(t *testing.T) {
		response := fixture.perform(requestSpec{
			method: http.MethodDelete, path: "/v1/runs/" + runID, token: adminToken,
		})
		assertStatus(t, response, http.StatusConflict)
	})

	t.Run("terminal run deletion succeeds once", func(t *testing.T) {
		if _, err := fixture.database.MigrationDB().ExecContext(
			t.Context(),
			`UPDATE eval_runs SET status = 'succeeded', finished_at = now() WHERE id = $1`,
			uuid.MustParse(runID),
		); err != nil {
			t.Fatalf("finish run: %v", err)
		}
		response := fixture.perform(requestSpec{
			method: http.MethodDelete, path: "/v1/runs/" + runID, token: adminToken,
		})
		assertStatus(t, response, http.StatusNoContent)
		response = fixture.perform(requestSpec{
			method: http.MethodDelete, path: "/v1/runs/" + runID, token: adminToken,
		})
		assertStatus(t, response, http.StatusNotFound)
	})

	t.Run("item survives run deletion", func(t *testing.T) {
		response := fixture.perform(requestSpec{
			method: http.MethodGet, path: "/v1/datasets/" + dataset + "/items/" + item,
			token: adminToken,
		})
		assertStatus(t, response, http.StatusOK)
	})
}

func TestAdminTraceScoreAndReferenceFailAtomically(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProjectNamed("Primary", "primary-secret")
	application := fixture.createApplication(project.ID)
	trace := fixture.ingestScorableTrace(project.ID, application.ID, 1)
	unknown := uuid.Must(uuid.NewV7())

	response := fixture.perform(requestSpec{
		method: http.MethodPost, path: "/v1/traces/score", token: adminToken,
		body: `{"trace_ids":["` + unknown.String() + `"],"scorers":["groundedness"]}`,
	})
	assertStatus(t, response, http.StatusNotFound)

	response = fixture.perform(requestSpec{
		method: http.MethodPatch,
		path:   "/v1/traces/" + unknown.String() + "/reference", token: adminToken,
		body: `{"reference_answer":"missing"}`,
	})
	assertStatus(t, response, http.StatusNotFound)

	if count := fixture.countTraceJobs(t, trace.ID); count != 0 {
		t.Fatalf("unknown batch created %d jobs", count)
	}
}

func (f *apiFixture) ingestScorableTrace(
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
			IsScorable:    true,
			Attributes: map[string]any{
				"gen_ai.input.messages": []any{
					map[string]any{"role": "user", "content": "question"},
				},
				"gen_ai.output.messages": []any{
					map[string]any{"role": "assistant", "content": "answer"},
				},
				"assay.context.chunk.count":   1,
				"assay.context.chunks.0.id":   "c0",
				"assay.context.chunks.0.text": "retrieved context",
				"gen_ai.input.tokens":         4,
				"gen_ai.output.tokens":        3,
			},
			Events: []domain.SpanEvent{},
		}},
	}
	if err := f.traces.Ingest(
		f.t.Context(), uuid.MustParse(projectID), []domain.Trace{trace},
	); err != nil {
		f.t.Fatalf("ingest scorable trace: %v", err)
	}
	return trace
}

func (f *apiFixture) createDatasetForMutation(applicationID string, name string) string {
	f.t.Helper()
	response := f.perform(requestSpec{
		method: http.MethodPost, path: "/v1/datasets", token: adminToken,
		body: `{"application_id":"` + applicationID + `","name":"` + name + `"}`,
	})
	assertStatus(f.t, response, http.StatusCreated)
	var dataset struct {
		ID string `json:"id"`
	}
	decodeResponse(f.t, response, &dataset)
	return dataset.ID
}

func (f *apiFixture) createDatasetItemForMutation(datasetID string, externalID string) string {
	f.t.Helper()
	response := f.perform(requestSpec{
		method: http.MethodPost, path: "/v1/datasets/" + datasetID + "/items",
		token: adminToken,
		body: `{"items":[{"external_id":"` + externalID + `","input":{"question":"original"},` +
			`"output":"answer","expected_output":"reference","context":[],"metadata":{}}]}`,
	})
	assertStatus(f.t, response, http.StatusCreated)
	var result struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	decodeResponse(f.t, response, &result)
	if len(result.Items) != 1 {
		f.t.Fatalf("created items = %#v", result.Items)
	}
	return result.Items[0].ID
}

func (f *apiFixture) createRunForMutation(applicationID string, datasetID string) string {
	f.t.Helper()
	response := f.perform(requestSpec{
		method: http.MethodPost, path: "/v1/runs", token: adminToken,
		body: `{"application_id":"` + applicationID + `","dataset_id":"` + datasetID +
			`","name":"snapshot","mode":"score_existing","scorers":["groundedness"]}`,
	})
	assertStatus(f.t, response, http.StatusAccepted)
	var run struct {
		ID string `json:"id"`
	}
	decodeResponse(f.t, response, &run)
	return run.ID
}

func (f *apiFixture) countTraceJobs(t *testing.T, traceID uuid.UUID) int {
	t.Helper()
	var count int
	if err := f.database.MigrationDB().QueryRowContext(
		t.Context(), `SELECT count(*) FROM jobs WHERE trace_id = $1`, traceID,
	).Scan(&count); err != nil {
		t.Fatalf("count trace jobs: %v", err)
	}
	return count
}
