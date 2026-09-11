package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

//nolint:cyclop // The route regression test verifies persisted evidence ordering and conflicts.
func TestDatasetItemFromTraceImportsLatestEvidenceOnce(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProject("synthetic-judge-secret")
	application := fixture.createApplication(project.ID)
	trace := fixture.ingestScorableTrace(project.ID, application.ID, 1)
	datasetID := createDatasetForMutation(t, fixture, application.ID, "regressions")
	stored, err := fixture.traces.GetAdmin(t.Context(), trace.ID)
	if err != nil {
		t.Fatalf("load trace evidence: %v", err)
	}
	if _, err := fixture.database.MigrationDB().ExecContext(t.Context(), `
INSERT INTO scores (
    scorer, value, threshold, passed, rationale, prompt_template_id, judge_model, judge_provider,
    trace_id, span_id, span_start_time, judged_input, judged_output,
    judged_context, judged_reference, created_at
) VALUES
    ('groundedness', 0.9, 0.7, true, 'test', 'test', 'test', 'test', $1, $2, $3,
     'first question', 'first answer', '[{"id":"k0","text":"evidence"}]'::jsonb,
     'first reference', '2026-09-01T12:00:00Z'),
    ('groundedness', 0.9, 0.7, true, 'test', 'test', 'test', 'test', $1, $2, $3,
     'latest question', 'latest answer', '[{"id":"k1","text":"latest evidence"}]'::jsonb,
     'latest reference', '2026-09-01T12:00:00Z')`,
		trace.ID, stored.Spans[0].ID, stored.Spans[0].StartTime,
	); err != nil {
		t.Fatalf("insert trace evidence: %v", err)
	}
	var latestScoreID int64
	if err := fixture.database.MigrationDB().QueryRowContext(
		t.Context(), `SELECT max(id) FROM scores WHERE trace_id = $1`, trace.ID,
	).Scan(&latestScoreID); err != nil {
		t.Fatalf("select latest score ID: %v", err)
	}
	path := "/v1/datasets/" + datasetID + "/from-trace"
	response := fixture.perform(requestSpec{
		method: http.MethodPost, path: path, token: adminToken,
		body: `{"trace_id":"` + trace.ID.String() + `","scorer":"groundedness",` +
			`"expected_output":" corrected "}`,
	})
	assertStatus(t, response, http.StatusCreated)
	var item struct {
		ExternalID     string         `json:"external_id"`
		ExpectedOutput string         `json:"expected_output"`
		Input          map[string]any `json:"input"`
		Output         string         `json:"output"`
		Metadata       map[string]any `json:"metadata"`
	}
	decodeResponse(t, response, &item)
	if item.ExternalID != "trace:"+trace.ID.String()+":groundedness" ||
		item.ExpectedOutput != "corrected" || item.Input["question"] != "latest question" ||
		item.Output != "latest answer" || item.Metadata["trace_id"] != trace.ID.String() ||
		item.Metadata["score_id"] != float64(latestScoreID) ||
		item.Metadata["scorer"] != "groundedness" {
		t.Fatalf("imported item = %#v", item)
	}
	assertStatus(t, fixture.perform(requestSpec{
		method: http.MethodPost, path: path, token: adminToken,
		body: `{"trace_id":"` + trace.ID.String() + `","scorer":"groundedness"}`,
	}), http.StatusConflict)
	unchanged := fixture.perform(requestSpec{
		method: http.MethodGet, path: "/v1/datasets/" + datasetID + "/items", token: adminToken,
	})
	assertStatus(t, unchanged, http.StatusOK)
	var page struct {
		Items []struct {
			Input          map[string]any `json:"input"`
			ExpectedOutput string         `json:"expected_output"`
		} `json:"items"`
	}
	decodeResponse(t, unchanged, &page)
	if len(page.Items) != 1 || page.Items[0].Input["question"] != "latest question" ||
		page.Items[0].ExpectedOutput != "corrected" {
		t.Fatalf("duplicate import changed items: %#v", page.Items)
	}
	otherProject := fixture.createProjectNamed("Other", "other-judge-secret")
	other := fixture.createApplication(otherProject.ID)
	foreignDataset := createDatasetForMutation(t, fixture, other.ID, "foreign")
	assertStatus(t, fixture.perform(requestSpec{
		method: http.MethodPost, path: "/v1/datasets/" + foreignDataset + "/from-trace",
		token: adminToken,
		body:  `{"trace_id":"` + trace.ID.String() + `","scorer":"groundedness"}`,
	}), http.StatusUnprocessableEntity)
}

func TestDatasetItemFromTraceRejectsMissingAndMalformedEvidence(t *testing.T) {
	t.Run("missing evidence", func(t *testing.T) {
		fixture := newAPIFixture(t)
		project := fixture.createProject("synthetic-judge-secret")
		application := fixture.createApplication(project.ID)
		trace := fixture.ingestScorableTrace(project.ID, application.ID, 1)
		datasetID := createDatasetForMutation(t, fixture, application.ID, "regressions")
		response := fixture.perform(requestSpec{
			method: http.MethodPost, path: "/v1/datasets/" + datasetID + "/from-trace",
			token: adminToken,
			body:  `{"trace_id":"` + trace.ID.String() + `","scorer":"groundedness"}`,
		})
		assertStatus(t, response, http.StatusUnprocessableEntity)
	})
	t.Run("malformed retained context", func(t *testing.T) {
		fixture := newAPIFixture(t)
		project := fixture.createProject("synthetic-judge-secret")
		application := fixture.createApplication(project.ID)
		trace := fixture.ingestScorableTrace(project.ID, application.ID, 2)
		datasetID := createDatasetForMutation(t, fixture, application.ID, "regressions")
		stored, err := fixture.traces.GetAdmin(t.Context(), trace.ID)
		if err != nil {
			t.Fatalf("load trace evidence: %v", err)
		}
		if _, err := fixture.database.MigrationDB().ExecContext(t.Context(), `
INSERT INTO scores (
    scorer, value, threshold, passed, rationale, prompt_template_id, judge_model, judge_provider,
    trace_id, span_id, span_start_time, judged_input, judged_output, judged_context
) VALUES (
    'groundedness', 0.9, 0.7, true, 'test', 'test', 'test', 'test', $1, $2, $3,
    'question', 'answer', '[{"id":"k0","text":1}]'::jsonb
)`, trace.ID, stored.Spans[0].ID, stored.Spans[0].StartTime); err != nil {
			t.Fatalf("insert malformed trace evidence: %v", err)
		}
		response := fixture.perform(requestSpec{
			method: http.MethodPost, path: "/v1/datasets/" + datasetID + "/from-trace",
			token: adminToken,
			body:  `{"trace_id":"` + trace.ID.String() + `","scorer":"groundedness"}`,
		})
		assertStatus(t, response, http.StatusUnprocessableEntity)
	})
}

func TestDatasetItemFromTraceUsesEvidenceAfterSpanDeletion(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProject("synthetic-judge-secret")
	application := fixture.createApplication(project.ID)
	trace := fixture.ingestScorableTrace(project.ID, application.ID, 3)
	datasetID := createDatasetForMutation(t, fixture, application.ID, "regressions")
	stored, err := fixture.traces.GetAdmin(t.Context(), trace.ID)
	if err != nil {
		t.Fatalf("load trace evidence: %v", err)
	}
	if _, err := fixture.database.MigrationDB().ExecContext(t.Context(), `
INSERT INTO scores (
    scorer, value, threshold, passed, rationale, prompt_template_id, judge_model, judge_provider,
    trace_id, span_id, span_start_time, judged_input, judged_output, judged_context,
    judged_reference
) VALUES (
    'groundedness', 0.9, 0.7, true, 'test', 'test', 'test', 'test', $1, $2, $3,
    'question', 'answer', '[{"id":"k0","text":"evidence"}]'::jsonb, 'reference'
)`, trace.ID, stored.Spans[0].ID, stored.Spans[0].StartTime); err != nil {
		t.Fatalf("insert trace evidence: %v", err)
	}
	if _, err := fixture.database.MigrationDB().ExecContext(
		t.Context(), `DELETE FROM spans WHERE id = $1`, stored.Spans[0].ID,
	); err != nil {
		t.Fatalf("delete scored span: %v", err)
	}
	response := fixture.perform(requestSpec{
		method: http.MethodPost, path: "/v1/datasets/" + datasetID + "/from-trace",
		token: adminToken,
		body:  `{"trace_id":"` + trace.ID.String() + `","scorer":"groundedness"}`,
	})
	assertStatus(t, response, http.StatusCreated)
	var item struct {
		ExpectedOutput string `json:"expected_output"`
	}
	decodeResponse(t, response, &item)
	if item.ExpectedOutput != "reference" {
		t.Fatalf("default expected output = %q, want reference", item.ExpectedOutput)
	}
}

func TestDatasetRename(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProject("synthetic-judge-secret")
	application := fixture.createApplication(project.ID)
	datasetID := createDatasetForMutation(t, fixture, application.ID, "baseline")

	renamed := fixture.perform(requestSpec{
		method: http.MethodPatch, path: "/v1/datasets/" + datasetID, token: adminToken,
		body: `{"name":"regression"}`,
	})
	assertStatus(t, renamed, http.StatusOK)
	var result struct {
		Name string `json:"name"`
	}
	decodeResponse(t, renamed, &result)
	if result.Name != "regression" {
		t.Fatalf("name = %q, want regression", result.Name)
	}
}

func TestDatasetMetadataValidation(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProject("synthetic-judge-secret")
	application := fixture.createApplication(project.ID)
	datasetID := createDatasetForMutation(t, fixture, application.ID, "baseline")
	createDatasetForMutation(t, fixture, application.ID, "duplicate")

	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "empty", body: `{}`, want: http.StatusUnprocessableEntity},
		{name: "blank name", body: `{"name":" "}`, want: http.StatusUnprocessableEntity},
		{name: "duplicate name", body: `{"name":"duplicate"}`, want: http.StatusConflict},
		{
			name: "set and clear description",
			body: `{"description":"new","clear_description":true}`,
			want: http.StatusUnprocessableEntity,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := fixture.perform(requestSpec{
				method: http.MethodPatch, path: "/v1/datasets/" + datasetID,
				token: adminToken, body: test.body,
			})
			assertStatus(t, response, test.want)
		})
	}
}

func TestDatasetItemReplacementSchemaRequiresNullableFields(t *testing.T) {
	handler := newDocumentationHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var document openAPIDocument
	decodeResponse(t, response, &document)
	schema := document.Components.Schemas["ReplaceDatasetItemInputBody"]
	wantRequired := []string{
		"external_id", "input", "output", "expected_output", "context", "metadata",
	}
	if !slices.Equal(schema.Required, wantRequired) {
		t.Fatalf("required replacement fields = %v, want %v", schema.Required, wantRequired)
	}
	if !bytes.Contains(schema.Properties["output"], []byte(`"null"`)) {
		t.Fatalf("output schema is not nullable: %s", schema.Properties["output"])
	}
	if bytes.Contains(schema.Properties["context"], []byte(`"null"`)) {
		t.Fatalf("context schema is nullable: %s", schema.Properties["context"])
	}
}

func TestDatasetItemReplacementAndScopedDeletion(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProject("synthetic-judge-secret")
	application := fixture.createApplication(project.ID)
	datasetID := createDatasetForMutation(t, fixture, application.ID, "first")
	otherDatasetID := createDatasetForMutation(t, fixture, application.ID, "second")
	itemID := createDatasetItemForMutation(t, fixture, datasetID, "case-1")
	createDatasetItemForMutation(t, fixture, datasetID, "case-2")
	runID := createRunForMutation(t, fixture, application.ID, datasetID)
	path := "/v1/datasets/" + datasetID + "/items/" + itemID

	conflict := fixture.perform(requestSpec{
		method: http.MethodPut, path: path, token: adminToken,
		body: `{"external_id":"case-2","input":{"question":"conflict"},` +
			`"output":null,"expected_output":null,"context":[],"metadata":{}}`,
	})
	assertStatus(t, conflict, http.StatusConflict)
	unchanged := fixture.perform(requestSpec{method: http.MethodGet, path: path, token: adminToken})
	assertStatus(t, unchanged, http.StatusOK)
	var original struct {
		Input map[string]any `json:"input"`
	}
	decodeResponse(t, unchanged, &original)
	if original.Input["question"] != "original" {
		t.Fatalf("item changed after conflict: %#v", original)
	}

	replaced := fixture.perform(requestSpec{
		method: http.MethodPut, path: path, token: adminToken,
		body: `{"external_id":null,"input":{"question":" changed "},` +
			`"output":null,"expected_output":" expected ","context":[],"metadata":{}}`,
	})
	assertStatus(t, replaced, http.StatusOK)
	var item struct {
		Input          map[string]any `json:"input"`
		Output         *string        `json:"output"`
		ExpectedOutput *string        `json:"expected_output"`
	}
	decodeResponse(t, replaced, &item)
	if item.Input["question"] != "changed" || item.Output != nil ||
		item.ExpectedOutput == nil || *item.ExpectedOutput != "expected" {
		t.Fatalf("replaced item = %#v", item)
	}

	wrongPath := "/v1/datasets/" + otherDatasetID + "/items/" + itemID
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		response := fixture.perform(requestSpec{method: method, path: wrongPath, token: adminToken})
		assertStatus(t, response, http.StatusNotFound)
	}
	wrongReplace := fixture.perform(requestSpec{
		method: http.MethodPut, path: wrongPath, token: adminToken,
		body: `{"external_id":null,"input":{"question":"wrong"},` +
			`"output":null,"expected_output":null,"context":[],"metadata":{}}`,
	})
	assertStatus(t, wrongReplace, http.StatusNotFound)

	deleted := fixture.perform(requestSpec{method: http.MethodDelete, path: path, token: adminToken})
	assertStatus(t, deleted, http.StatusNoContent)
	missing := fixture.perform(requestSpec{method: http.MethodGet, path: path, token: adminToken})
	assertStatus(t, missing, http.StatusNotFound)
	assertAPIRunSnapshot(t, fixture, runID, itemID)
}

func createRunForMutation(
	t *testing.T,
	fixture *apiFixture,
	applicationID string,
	datasetID string,
) string {
	t.Helper()
	response := fixture.perform(requestSpec{
		method: http.MethodPost, path: "/v1/runs", token: adminToken,
		body: `{"application_id":"` + applicationID + `","dataset_id":"` + datasetID +
			`","name":"snapshot","mode":"score_existing","scorers":["groundedness"]}`,
	})
	assertStatus(t, response, http.StatusAccepted)
	var run struct {
		ID string `json:"id"`
	}
	decodeResponse(t, response, &run)
	return run.ID
}

func assertAPIRunSnapshot(t *testing.T, fixture *apiFixture, runID string, itemID string) {
	t.Helper()
	response := fixture.perform(requestSpec{
		method: http.MethodGet, path: "/v1/runs/" + runID + "/items", token: adminToken,
	})
	assertStatus(t, response, http.StatusOK)
	var page struct {
		Items []struct {
			DatasetItemID  string `json:"dataset_item_id"`
			SnapshotOrigin string `json:"snapshot_origin"`
			Snapshot       struct {
				Input map[string]any `json:"input"`
			} `json:"snapshot"`
		} `json:"items"`
	}
	decodeResponse(t, response, &page)
	for _, item := range page.Items {
		if item.DatasetItemID == itemID && item.SnapshotOrigin == "creation" &&
			item.Snapshot.Input["question"] == "original" {
			return
		}
	}
	t.Fatalf("original run snapshot missing from %#v", page.Items)
}

func createDatasetForMutation(
	t *testing.T,
	fixture *apiFixture,
	applicationID string,
	name string,
) string {
	t.Helper()
	response := fixture.perform(requestSpec{
		method: http.MethodPost, path: "/v1/datasets", token: adminToken,
		body: `{"application_id":"` + applicationID + `","name":"` + name + `"}`,
	})
	assertStatus(t, response, http.StatusCreated)
	var dataset struct {
		ID string `json:"id"`
	}
	decodeResponse(t, response, &dataset)
	return dataset.ID
}

func createDatasetItemForMutation(
	t *testing.T,
	fixture *apiFixture,
	datasetID string,
	externalID string,
) string {
	t.Helper()
	response := fixture.perform(requestSpec{
		method: http.MethodPost, path: "/v1/datasets/" + datasetID + "/items",
		token: adminToken,
		body: `{"items":[{"external_id":"` + externalID + `","input":{"question":"original"},` +
			`"output":"answer","expected_output":"reference","context":[],"metadata":{}}]}`,
	})
	assertStatus(t, response, http.StatusCreated)
	var result struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	decodeResponse(t, response, &result)
	if len(result.Items) != 1 {
		t.Fatalf("created items = %#v", result.Items)
	}
	return result.Items[0].ID
}
