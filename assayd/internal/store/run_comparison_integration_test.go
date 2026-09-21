package store

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestRunComparisonUsesHistoricalCasesAndAllPageSummary(t *testing.T) {
	database := openSnapshotTraceDatabase(t, nil)
	fixture := createComparisonFixture(t, database)
	service := domain.NewRunComparisonService(database)

	first, err := service.Compare(t.Context(), domain.RunComparisonQuery{
		BaselineID: fixture.baselineID, CandidateID: fixture.candidateID,
		Scorer: domain.ScorerGroundedness, Limit: 3,
	})
	if err != nil {
		t.Fatalf("compare first page: %v", err)
	}
	assertFirstComparisonPage(t, first, fixture.itemIDs[2])

	second, err := service.Compare(t.Context(), domain.RunComparisonQuery{
		BaselineID: fixture.baselineID, CandidateID: fixture.candidateID,
		Scorer: domain.ScorerGroundedness, Limit: 4, Cursor: first.NextCursor,
	})
	if err != nil {
		t.Fatalf("compare second page: %v", err)
	}
	assertSecondComparisonPage(t, second)

	if err := database.DeleteEvalRun(t.Context(), fixture.candidateID); err != nil {
		t.Fatalf("delete candidate run: %v", err)
	}
	_, err = database.CompareEvalRuns(t.Context(), domain.RunComparisonQuery{
		BaselineID: fixture.baselineID, CandidateID: fixture.candidateID,
		Scorer: domain.ScorerGroundedness, Limit: 4,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("compare deleted pair error = %v, want not found", err)
	}
}

func assertFirstComparisonPage(
	t *testing.T,
	page domain.RunComparisonPage,
	wantCursor uuid.UUID,
) {
	t.Helper()
	if len(page.Items) != 3 || page.NextCursor == nil || *page.NextCursor != wantCursor {
		t.Fatalf("first page items/cursor = %d / %v", len(page.Items), page.NextCursor)
	}
	assertComparisonKinds(t, page.Items, []string{
		domain.ComparisonMatched, domain.ComparisonMatched, domain.ComparisonMatched,
	})
	assertComparisonSummary(t, page.Summary)
	assertComparisonWarningsAndZero(t, page)
}

func assertSecondComparisonPage(t *testing.T, page domain.RunComparisonPage) {
	t.Helper()
	assertComparisonKinds(t, page.Items, []string{
		domain.ComparisonCandidateOnly, domain.ComparisonBaselineOnly,
		domain.ComparisonUnscored, domain.ComparisonChangedCase,
	})
	if page.NextCursor != nil {
		t.Fatalf("second page cursor = %v", page.NextCursor)
	}
	assertComparisonSummary(t, page.Summary)
	assertExcludedComparisonEvidence(t, page.Items)
}

func assertComparisonSummary(t *testing.T, summary domain.RunComparisonSummary) {
	t.Helper()
	if summary.N != 3 || summary.MeanDelta == nil ||
		math.Abs(*summary.MeanDelta-(-0.1/3)) > 1e-12 {
		t.Fatalf("comparison summary = %+v", summary)
	}
}

func assertComparisonWarningsAndZero(t *testing.T, page domain.RunComparisonPage) {
	t.Helper()
	if len(page.Warnings) != 2 || page.Items[2].Delta == nil || *page.Items[2].Delta != 0 {
		t.Fatalf("warnings/zero delta = %v / %v", page.Warnings, page.Items[2].Delta)
	}
}

func assertExcludedComparisonEvidence(t *testing.T, items []domain.RunComparisonRow) {
	t.Helper()
	if items[0].Baseline != nil || items[1].Candidate != nil ||
		items[2].Delta != nil || items[3].Delta != nil {
		t.Fatalf("excluded comparison evidence = %+v", items)
	}
}

type comparisonFixture struct {
	baselineID  uuid.UUID
	candidateID uuid.UUID
	itemIDs     []uuid.UUID
}

func createComparisonFixture(t *testing.T, database *Database) comparisonFixture {
	t.Helper()
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	applicationID := uuid.MustParse("00000000-0000-0000-0000-000000000102")
	datasetID := uuid.MustParse("00000000-0000-0000-0000-000000000103")
	baselineID := uuid.MustParse("00000000-0000-0000-0000-000000000104")
	candidateID := uuid.MustParse("00000000-0000-0000-0000-000000000105")
	itemIDs := make([]uuid.UUID, 7)
	for index := range itemIDs {
		itemIDs[index] = uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-0000000002%02d", index+1))
	}
	mustExecSnapshot(t, database, "insert comparison project", `
INSERT INTO projects (id, name) VALUES ($1, 'Comparison')`, projectID)
	mustExecSnapshot(t, database, "insert comparison application", `
INSERT INTO applications (id, project_id, name, slug)
VALUES ($1, $2, 'Comparison', 'comparison')`, applicationID, projectID)
	mustExecSnapshot(t, database, "insert comparison dataset", `
INSERT INTO datasets (id, application_id, name) VALUES ($1, $2, 'Cases')`, datasetID, applicationID)
	for _, runID := range []uuid.UUID{baselineID, candidateID} {
		mustExecSnapshot(t, database, "insert comparison run", `
INSERT INTO eval_runs (
    id, application_id, dataset_id, name, status, mode, scorers, total_items
) VALUES ($1, $2, $3, 'Comparison', 'succeeded', 'score_existing', ARRAY['groundedness'], 6)`,
			runID, applicationID, datasetID)
	}
	insertComparisonItems(t, database, baselineID, candidateID, datasetID, itemIDs)
	insertComparisonScores(t, database, baselineID, candidateID, itemIDs)
	return comparisonFixture{baselineID: baselineID, candidateID: candidateID, itemIDs: itemIDs}
}

func insertComparisonItems(
	t *testing.T,
	database *Database,
	baselineID uuid.UUID,
	candidateID uuid.UUID,
	datasetID uuid.UUID,
	itemIDs []uuid.UUID,
) {
	t.Helper()
	for index, itemID := range itemIDs {
		if index != 3 {
			insertComparisonItem(
				t, database, baselineID, itemID, datasetID,
				"succeeded", "q", "[]", "baseline",
			)
		}
		if index != 4 {
			status := "succeeded"
			question := "q"
			context := "[]"
			if index == 0 {
				context = `[{"id":"new","text":"changed"}]`
			}
			if index == 5 {
				status = "failed"
			}
			if index == 6 {
				question = "changed"
			}
			insertComparisonItem(
				t, database, candidateID, itemID, datasetID,
				status, question, context, "candidate",
			)
		}
	}
}

func insertComparisonItem(
	t *testing.T,
	database *Database,
	runID uuid.UUID,
	itemID uuid.UUID,
	datasetID uuid.UUID,
	status string,
	question string,
	context string,
	externalID string,
) {
	t.Helper()
	mustExecSnapshot(t, database, "insert comparison item", `
INSERT INTO eval_run_items (
    eval_run_id, dataset_item_id, status, snapshot_dataset_id, snapshot_external_id,
    snapshot_input, snapshot_output, snapshot_expected_output, snapshot_context,
    snapshot_metadata, snapshot_created_at, snapshot_updated_at, snapshot_origin
) VALUES ($1, $2, $3, $4, $5, jsonb_build_object('question', $6::text), 'answer',
          'reference', $7::jsonb, '{}'::jsonb, now(), now(), 'creation')`,
		runID, itemID, status, datasetID, externalID, question, context)
}

func insertComparisonScores(
	t *testing.T,
	database *Database,
	baselineID uuid.UUID,
	candidateID uuid.UUID,
	itemIDs []uuid.UUID,
) {
	t.Helper()
	insertComparisonScore(t, database, baselineID, itemIDs[0], 0.1, "judge-a")
	insertComparisonScore(t, database, baselineID, itemIDs[0], 0.4, "judge-a")
	insertComparisonScore(t, database, candidateID, itemIDs[0], 0.9, "judge-a")
	insertComparisonScore(t, database, baselineID, itemIDs[1], 0.8, "judge-a")
	insertComparisonScore(t, database, candidateID, itemIDs[1], 0.2, "judge-b")
	insertComparisonScore(t, database, baselineID, itemIDs[2], 0, "judge-a")
	insertComparisonScore(t, database, candidateID, itemIDs[2], 0, "judge-a")
	insertComparisonScore(t, database, candidateID, itemIDs[3], 0.9, "judge-a")
	insertComparisonScore(t, database, baselineID, itemIDs[4], 0.4, "judge-a")
	insertComparisonScore(t, database, baselineID, itemIDs[5], 0.4, "judge-a")
	insertComparisonScore(t, database, baselineID, itemIDs[6], 0.4, "judge-a")
	insertComparisonScore(t, database, candidateID, itemIDs[6], 0.9, "judge-a")
}

func insertComparisonScore(
	t *testing.T,
	database *Database,
	runID uuid.UUID,
	itemID uuid.UUID,
	value float64,
	model string,
) {
	t.Helper()
	mustExecSnapshot(t, database, "insert comparison score", `
INSERT INTO scores (
    scorer, value, threshold, passed, rationale, details, prompt_template_id,
    judge_model, judge_provider, eval_run_id, dataset_item_id
) VALUES ('groundedness', $3, 0.5, $3 >= 0.5, 'evidence', '{}'::jsonb,
          'groundedness@v1', $4, 'fake', $1, $2)`, runID, itemID, value, model)
}

func assertComparisonKinds(t *testing.T, items []domain.RunComparisonRow, want []string) {
	t.Helper()
	if len(items) != len(want) {
		t.Fatalf("item count = %d, want %d", len(items), len(want))
	}
	for index, kind := range want {
		if items[index].Kind != kind {
			t.Fatalf("item %d kind = %q, want %q", index, items[index].Kind, kind)
		}
	}
}
