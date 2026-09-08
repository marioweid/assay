package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

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
