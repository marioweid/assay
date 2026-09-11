package store_test

import (
	"errors"
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestDatasetItemMutationsAreParentScopedAndPreserveRunSnapshot(t *testing.T) {
	_, evaluations, runID, itemID := createLeaseFixture(t)
	run, err := evaluations.GetEvalRun(t.Context(), runID)
	if err != nil {
		t.Fatalf("get run fixture: %v", err)
	}
	other, err := evaluations.CreateDataset(t.Context(), domain.CreateDatasetInput{
		ApplicationID: run.ApplicationID, Name: "other-cases",
	})
	if err != nil {
		t.Fatalf("create other dataset: %v", err)
	}
	assertDatasetItemNotFound(t, evaluations, other.ID, itemID)
	replaceDatasetItemFixture(t, evaluations, run.DatasetID, itemID)
	assertRunSnapshotQuestion(t, evaluations, runID, "question")

	if err := evaluations.DeleteDatasetItem(t.Context(), run.DatasetID, itemID); err != nil {
		t.Fatalf("delete dataset item: %v", err)
	}
	assertDatasetItemNotFound(t, evaluations, run.DatasetID, itemID)
	assertRunSnapshotQuestion(t, evaluations, runID, "question")
}

func assertDatasetItemNotFound(
	t *testing.T,
	evaluations *domain.EvaluationService,
	datasetID uuid.UUID,
	itemID uuid.UUID,
) {
	t.Helper()
	if _, err := evaluations.GetDatasetItem(t.Context(), datasetID, itemID); !errors.Is(
		err, domain.ErrNotFound,
	) {
		t.Fatalf("dataset item lookup error = %v, want ErrNotFound", err)
	}
}

func replaceDatasetItemFixture(
	t *testing.T,
	evaluations *domain.EvaluationService,
	datasetID uuid.UUID,
	itemID uuid.UUID,
) {
	t.Helper()
	expected := "new reference"
	replaced, err := evaluations.ReplaceDatasetItem(t.Context(), datasetID, itemID,
		domain.ReplaceDatasetItemInput{
			Input: map[string]any{"question": "new question"}, Output: nil,
			ExpectedOutput: &expected, Context: []domain.Chunk{}, Metadata: map[string]any{},
		})
	if err != nil {
		t.Fatalf("replace dataset item: %v", err)
	}
	if replaced.ID != itemID || replaced.CreatedAt.IsZero() || replaced.Output != nil {
		t.Fatalf("replaced dataset item = %#v", replaced)
	}
}

func assertRunSnapshotQuestion(
	t *testing.T,
	evaluations *domain.EvaluationService,
	runID uuid.UUID,
	want string,
) {
	t.Helper()
	page, err := evaluations.ListEvalRunItems(t.Context(), runID, domain.PageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list run snapshots: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Item.Input["question"] != want {
		t.Fatalf("run snapshots = %#v, want question %q", page.Items, want)
	}
}
