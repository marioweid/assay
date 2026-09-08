package store_test

import (
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"
)

func TestRunSnapshotsSurviveSourceMutation(t *testing.T) {
	database, evaluations, runID, itemID := createLeaseFixture(t)
	if _, err := database.MigrationDB().ExecContext(
		t.Context(),
		`UPDATE dataset_items SET input = '{"question":"changed"}', output = 'changed' WHERE id = $1`,
		itemID,
	); err != nil {
		t.Fatalf("mutate source item: %v", err)
	}

	items, err := evaluations.ListEvalRunItems(t.Context(), runID, domain.PageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list run items: %v", err)
	}
	assertOriginalRunItem(t, items)

	if _, err := database.MigrationDB().ExecContext(
		t.Context(), "DELETE FROM dataset_items WHERE id = $1", itemID,
	); err != nil {
		t.Fatalf("delete source item: %v", err)
	}
	items, err = evaluations.ListEvalRunItems(t.Context(), runID, domain.PageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list run items after source deletion: %v", err)
	}
	if len(items.Items) != 1 {
		t.Fatalf("run items after source deletion = %#v", items)
	}
}

func assertOriginalRunItem(t *testing.T, page domain.EvalRunItemPage) {
	t.Helper()
	if len(page.Items) != 1 || page.Items[0].Item.Input["question"] != "question" ||
		page.Items[0].Item.Output == nil || *page.Items[0].Item.Output != "answer" {
		t.Fatalf("run items after source update = %#v", page)
	}
}
