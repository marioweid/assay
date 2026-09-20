package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type runItemResult struct {
	DatasetItemID string  `json:"dataset_item_id"`
	Status        string  `json:"status"`
	Error         *string `json:"error"`
	Snapshot      struct {
		Input   map[string]any `json:"input"`
		Context []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"context"`
	} `json:"snapshot"`
	SnapshotOrigin  string  `json:"snapshot_origin"`
	GeneratedOutput *string `json:"generated_output"`
	Scores          []struct {
		Scorer    string  `json:"scorer"`
		Value     float64 `json:"value"`
		Passed    bool    `json:"passed"`
		Rationale string  `json:"rationale"`
	} `json:"scores"`
}

func TestEvalRunItemsIncludeSnapshotsScoresAndScopedLookup(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProject("never-return-this-key")
	application := fixture.createApplication(project.ID)
	datasetID := createDatasetForMutation(t, fixture, application.ID, "review")
	itemIDs := createRunReviewItems(t, fixture, datasetID)
	runID := createRunForMutation(t, fixture, application.ID, datasetID)
	persistRunReviewResults(t, fixture, runID, itemIDs)

	response := fixture.perform(requestSpec{
		method: http.MethodGet, path: "/v1/runs/" + runID + "/items", token: adminToken,
	})
	assertStatus(t, response, http.StatusOK)
	assertRedacted(t, response.Body.String(), "never-return-this-key")
	if !strings.Contains(response.Body.String(), `"scores":[]`) {
		t.Fatalf("empty scores serialized as null: %s", response.Body.String())
	}
	var page struct {
		Items []runItemResult `json:"items"`
	}
	decodeResponse(t, response, &page)
	assertRunReviewResults(t, page.Items, itemIDs)

	assertStatus(t, fixture.perform(requestSpec{
		method: http.MethodDelete,
		path:   "/v1/datasets/" + datasetID + "/items/" + itemIDs[1],
		token:  adminToken,
	}), http.StatusNoContent)
	itemPath := "/v1/runs/" + runID + "/items/" + itemIDs[1]
	assertStatus(t, fixture.perform(requestSpec{method: http.MethodGet, path: itemPath}),
		http.StatusUnauthorized)
	direct := fixture.perform(requestSpec{method: http.MethodGet, path: itemPath, token: adminToken})
	assertStatus(t, direct, http.StatusOK)
	var item runItemResult
	decodeResponse(t, direct, &item)
	if item.DatasetItemID != itemIDs[1] || len(item.Scores) != 2 || item.Scores[1].Value != 0 {
		t.Fatalf("direct run item = %#v", item)
	}

	unknown := uuid.Must(uuid.NewV7()).String()
	for _, path := range []string{
		"/v1/runs/" + unknown + "/items/" + itemIDs[1],
		"/v1/runs/" + runID + "/items/" + unknown,
	} {
		assertStatus(t, fixture.perform(requestSpec{
			method: http.MethodGet, path: path, token: adminToken,
		}), http.StatusNotFound)
	}
}

func createRunReviewItems(t *testing.T, fixture *apiFixture, datasetID string) []string {
	t.Helper()
	response := fixture.perform(requestSpec{
		method: http.MethodPost, path: "/v1/datasets/" + datasetID + "/items", token: adminToken,
		body: `{"items":[` +
			`{"external_id":"pass","input":{"question":"pass"},"output":"yes",` +
			`"expected_output":"yes","context":[{"id":"資料","text":"根拠"}],"metadata":{}},` +
			`{"external_id":"zero","input":{"question":"zero"},"output":"no",` +
			`"expected_output":"yes","context":[],"metadata":{}},` +
			`{"external_id":"error","input":{"question":"error"},"output":"no",` +
			`"expected_output":"yes","context":[],"metadata":{}}]}`,
	})
	assertStatus(t, response, http.StatusCreated)
	var result struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	decodeResponse(t, response, &result)
	if len(result.Items) != 3 {
		t.Fatalf("created run review items = %#v", result.Items)
	}
	return []string{result.Items[0].ID, result.Items[1].ID, result.Items[2].ID}
}

func persistRunReviewResults(t *testing.T, fixture *apiFixture, runID string, itemIDs []string) {
	t.Helper()
	failure := "target request failed"
	_, err := fixture.database.MigrationDB().ExecContext(t.Context(), `
UPDATE eval_run_items
SET status = CASE WHEN dataset_item_id = $4 THEN 'failed' ELSE 'succeeded' END,
    error = CASE WHEN dataset_item_id = $4 THEN $5 ELSE NULL END,
    generated_output = CASE WHEN dataset_item_id = $2 THEN 'generated pass' ELSE NULL END,
    finished_at = now(), updated_at = now()
WHERE eval_run_id = $1 AND dataset_item_id IN ($2, $3, $4)`,
		runID, itemIDs[0], itemIDs[1], itemIDs[2], failure)
	if err != nil {
		t.Fatalf("persist run item outcomes: %v", err)
	}
	_, err = fixture.database.MigrationDB().ExecContext(t.Context(), `
INSERT INTO scores (
    scorer, value, threshold, passed, rationale, details, prompt_template_id,
    judge_model, judge_provider, judge_tokens, eval_run_id, dataset_item_id
) VALUES
    ('groundedness', 0.8, 0.5, true, 'supported', '{"claims":[]}',
     'groundedness@v1', 'judge', 'fake', 7, $1, $2),
    ('correctness', 0.2, 0.5, false, 'incorrect', '{"facts":[]}',
     'correctness@v1', 'judge', 'fake', 8, $1, $3),
    ('groundedness', 0, 0.5, false, 'unsupported', '{"claims":[]}',
     'groundedness@v1', 'judge', 'fake', 9, $1, $3)`, runID, itemIDs[0], itemIDs[1])
	if err != nil {
		t.Fatalf("persist run item scores: %v", err)
	}
}

func assertRunReviewResults(t *testing.T, items []runItemResult, itemIDs []string) {
	t.Helper()
	byID := make(map[string]runItemResult, len(items))
	for _, item := range items {
		byID[item.DatasetItemID] = item
	}
	assertPassingRunItem(t, byID[itemIDs[0]])
	assertZeroScoreRunItem(t, byID[itemIDs[1]])
	assertFailedRunItem(t, byID[itemIDs[2]])
}

func assertPassingRunItem(t *testing.T, item runItemResult) {
	t.Helper()
	if len(item.Scores) != 1 || item.GeneratedOutput == nil || len(item.Snapshot.Context) != 1 ||
		item.Snapshot.Context[0].Text != "根拠" {
		t.Fatalf("passing item = %#v", item)
	}
}

func assertZeroScoreRunItem(t *testing.T, item runItemResult) {
	t.Helper()
	if item.Status != "succeeded" || len(item.Scores) != 2 ||
		item.Scores[0].Scorer != "correctness" || item.Scores[1].Value != 0 ||
		item.Scores[1].Passed {
		t.Fatalf("zero-score item = %#v", item)
	}
}

func assertFailedRunItem(t *testing.T, item runItemResult) {
	t.Helper()
	if item.Status != "failed" || item.Error == nil || len(item.Scores) != 0 {
		t.Fatalf("execution failure item = %#v", item)
	}
}
