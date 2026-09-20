package store_test

import (
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"
)

func TestRunItemReadsReturnAllScoresAfterSourceDeletion(t *testing.T) {
	database, evaluations, runID, itemID := createLeaseFixture(t)
	_, err := database.MigrationDB().ExecContext(t.Context(), `
INSERT INTO scores (
    scorer, value, threshold, passed, rationale, details, prompt_template_id,
    judge_model, judge_provider, judge_tokens, eval_run_id, dataset_item_id, created_at
)
SELECT 'groundedness', 0, 0.5, false, 'score-' || n, '{}'::jsonb,
       'groundedness@v1', 'judge', 'fake', n, $1, $2,
       now() + n * interval '1 microsecond'
FROM generate_series(1, 101) AS n`, runID, itemID)
	requireStoreSuccess(t, err)
	_, err = database.MigrationDB().ExecContext(
		t.Context(), "DELETE FROM dataset_items WHERE id = $1", itemID,
	)
	requireStoreSuccess(t, err)

	page, err := evaluations.ListEvalRunItems(
		t.Context(), runID, domain.PageQuery{Limit: 1},
	)
	requireStoreSuccess(t, err)
	item, err := evaluations.GetEvalRunItem(t.Context(), runID, itemID)
	requireStoreSuccess(t, err)
	if len(page.Items) != 1 || len(page.Items[0].Scores) != 101 || len(item.Scores) != 101 {
		t.Fatalf(
			"run item score counts = page %d, direct %d",
			len(page.Items[0].Scores), len(item.Scores),
		)
	}
	if item.Scores[0].ID >= item.Scores[100].ID || item.Item.Input["question"] != "question" {
		t.Fatalf("run item result order or snapshot = %#v", item)
	}
}
