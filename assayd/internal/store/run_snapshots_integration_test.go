package store_test

import (
	"errors"
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
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

func TestRunCreationRejectsInvalidCopiedSnapshot(t *testing.T) {
	database, evaluations, fixtureRunID, itemID := createLeaseFixture(t)
	fixtureRun, err := evaluations.GetEvalRun(t.Context(), fixtureRunID)
	if err != nil {
		t.Fatalf("get fixture run: %v", err)
	}
	if _, err := database.MigrationDB().ExecContext(
		t.Context(), "UPDATE dataset_items SET output = NULL WHERE id = $1", itemID,
	); err != nil {
		t.Fatalf("invalidate source item: %v", err)
	}
	runID := uuid.Must(uuid.NewV7())
	jobID := uuid.Must(uuid.NewV7())
	_, err = database.CreateEvalRun(t.Context(), domain.EvalRun{
		ID: runID, ApplicationID: fixtureRun.ApplicationID, DatasetID: fixtureRun.DatasetID,
		Name: "invalid-snapshot", Mode: domain.EvalModeScoreExisting,
		Params: map[string]any{}, Scorers: []string{domain.ScorerGroundedness},
	}, domain.Job{ID: jobID, EvalRunID: &runID, MaxAttempts: 3})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("invalid snapshot run error = %v, want ErrInvalid", err)
	}
	if _, err := database.GetEvalRun(t.Context(), runID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("rolled-back run lookup error = %v, want ErrNotFound", err)
	}
}

func assertOriginalRunItem(t *testing.T, page domain.EvalRunItemPage) {
	t.Helper()
	if len(page.Items) != 1 || page.Items[0].Item.Input["question"] != "question" ||
		page.Items[0].SnapshotOrigin != "creation" || page.Items[0].Item.Output == nil ||
		*page.Items[0].Item.Output != "answer" {
		t.Fatalf("run items after source update = %#v", page)
	}
}
