package api_test

import (
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func TestRunComparisonOpenAPIRequiresSelection(t *testing.T) {
	handler := newDocumentationHandler(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	var document openAPIDocument
	decodeResponse(t, response, &document)

	required := map[string]bool{}
	for _, parameter := range document.Paths["/v1/runs/{id}/comparison"]["get"].Parameters {
		required[parameter.Name] = parameter.Required
	}
	if !required["other_run_id"] || !required["scorer"] {
		t.Fatalf("required comparison parameters = %v", required)
	}
}

func TestRunComparisonReturnsPairedEvidenceAndBoundCursor(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProject("comparison-secret")
	application := fixture.createApplication(project.ID)
	datasetID := createDatasetForMutation(t, fixture, application.ID, "comparison")
	itemIDs := []string{
		createDatasetItemForMutation(t, fixture, datasetID, "first"),
		createDatasetItemForMutation(t, fixture, datasetID, "second"),
	}
	baselineID := createRunForMutation(t, fixture, application.ID, datasetID)
	candidateID := createRunForMutation(t, fixture, application.ID, datasetID)
	completeComparisonAPIRun(t, fixture, baselineID, itemIDs, []float64{0, 0.8})
	completeComparisonAPIRun(t, fixture, candidateID, itemIDs, []float64{0, 0.2})

	path := comparisonPath(baselineID, candidateID, "groundedness") + "&limit=1"
	response := fixture.perform(requestSpec{method: http.MethodGet, path: path, token: adminToken})
	assertStatus(t, response, http.StatusOK)
	var result comparisonAPIResponse
	decodeResponse(t, response, &result)
	assertComparisonAPIPage(t, result, baselineID, candidateID)
	assertComparisonAPISummary(t, result)

	swapped := comparisonPath(candidateID, baselineID, "groundedness") +
		"&cursor=" + url.QueryEscape(result.NextCursor)
	assertStatus(t, fixture.perform(requestSpec{
		method: http.MethodGet, path: swapped, token: adminToken,
	}), http.StatusUnprocessableEntity)
	assertStatus(t, fixture.perform(requestSpec{
		method: http.MethodGet,
		path:   comparisonPath(baselineID, candidateID, "groundedness") + "&cursor=invalid",
		token:  adminToken,
	}), http.StatusUnprocessableEntity)
}

type comparisonAPIResponse struct {
	BaselineRunID  string              `json:"baseline_run_id"`
	CandidateRunID string              `json:"candidate_run_id"`
	Items          []comparisonAPIItem `json:"items"`
	NextCursor     string              `json:"next_cursor"`
	Summary        struct {
		N         int      `json:"n"`
		MeanDelta *float64 `json:"mean_delta"`
	} `json:"summary"`
	Warnings []string `json:"warnings"`
}

type comparisonAPIItem struct {
	Kind      string   `json:"kind"`
	Delta     *float64 `json:"delta"`
	Baseline  any      `json:"baseline"`
	Candidate any      `json:"candidate"`
}

func assertComparisonAPIPage(
	t *testing.T,
	result comparisonAPIResponse,
	baselineID string,
	candidateID string,
) {
	t.Helper()
	if result.BaselineRunID != baselineID || result.CandidateRunID != candidateID {
		t.Fatalf("comparison IDs = %s / %s", result.BaselineRunID, result.CandidateRunID)
	}
	if len(result.Items) != 1 {
		t.Fatalf("comparison items = %#v", result.Items)
	}
	assertComparisonAPIItem(t, result.Items[0])
}

func assertComparisonAPIItem(t *testing.T, item comparisonAPIItem) {
	t.Helper()
	if item.Kind != "matched" || item.Delta == nil || *item.Delta != 0 {
		t.Fatalf("comparison kind/delta = %s / %v", item.Kind, item.Delta)
	}
	if item.Baseline == nil || item.Candidate == nil {
		t.Fatalf("comparison evidence = %#v", item)
	}
}

func assertComparisonAPISummary(t *testing.T, result comparisonAPIResponse) {
	t.Helper()
	if result.NextCursor == "" || result.Summary.N != 2 {
		t.Fatalf("comparison cursor/count = %q / %d", result.NextCursor, result.Summary.N)
	}
	if result.Summary.MeanDelta == nil ||
		math.Abs(*result.Summary.MeanDelta-(-0.3)) > 1e-12 {
		t.Fatalf("comparison mean = %v", result.Summary.MeanDelta)
	}
	if result.Warnings == nil {
		t.Fatal("comparison warnings must be a non-null array")
	}
}

func TestRunComparisonRejectsIncompatibleSelections(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProject("comparison-secret")
	application := fixture.createApplication(project.ID)
	datasetID := createDatasetForMutation(t, fixture, application.ID, "comparison")
	createDatasetItemForMutation(t, fixture, datasetID, "case")
	baselineID := createRunForMutation(t, fixture, application.ID, datasetID)
	candidateID := createRunForMutation(t, fixture, application.ID, datasetID)

	assertStatus(t, fixture.perform(requestSpec{
		method: http.MethodGet, path: comparisonPath(baselineID, candidateID, "groundedness"),
		token: adminToken,
	}), http.StatusConflict)
	assertStatus(t, fixture.perform(requestSpec{
		method: http.MethodGet, path: comparisonPath(baselineID, baselineID, "groundedness"),
		token: adminToken,
	}), http.StatusUnprocessableEntity)
}

func comparisonPath(baselineID, candidateID, scorer string) string {
	return "/v1/runs/" + baselineID + "/comparison?other_run_id=" + candidateID +
		"&scorer=" + scorer
}

func completeComparisonAPIRun(
	t *testing.T,
	fixture *apiFixture,
	runID string,
	itemIDs []string,
	values []float64,
) {
	t.Helper()
	parsedRunID := uuid.MustParse(runID)
	_, err := fixture.database.MigrationDB().ExecContext(t.Context(), `
UPDATE eval_runs
SET status = 'succeeded', succeeded_items = total_items, finished_at = now(), updated_at = now()
WHERE id = $1`, parsedRunID)
	if err != nil {
		t.Fatalf("complete comparison run: %v", err)
	}
	for index, itemID := range itemIDs {
		parsedItemID := uuid.MustParse(itemID)
		_, err = fixture.database.MigrationDB().ExecContext(t.Context(), `
UPDATE eval_run_items
SET status = 'succeeded', finished_at = now(), updated_at = now()
WHERE eval_run_id = $1 AND dataset_item_id = $2`, parsedRunID, parsedItemID)
		if err != nil {
			t.Fatalf("complete comparison item: %v", err)
		}
		_, err = fixture.database.MigrationDB().ExecContext(t.Context(), `
INSERT INTO scores (
    scorer, value, threshold, passed, rationale, details, prompt_template_id,
    judge_model, judge_provider, eval_run_id, dataset_item_id
) VALUES ('groundedness', $3, 0.5, $3 >= 0.5, 'evidence', '{}'::jsonb,
          'groundedness@v1', 'judge', 'fake', $1, $2)`, parsedRunID, parsedItemID, values[index])
		if err != nil {
			t.Fatalf("insert comparison score: %v", err)
		}
	}
}
