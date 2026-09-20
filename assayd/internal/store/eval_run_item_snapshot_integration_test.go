package store

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestEvalRunItemReadsUseOneSnapshot(t *testing.T) {
	for _, direct := range []bool{false, true} {
		name := "list"
		if direct {
			name = "direct"
		}
		t.Run(name, func(t *testing.T) {
			assertEvalRunItemReadSnapshot(t, direct)
		})
	}
}

func assertEvalRunItemReadSnapshot(t *testing.T, direct bool) {
	t.Helper()
	blocker := newRunItemScoreBlocker()
	database := openSnapshotTraceDatabase(t, blocker)
	runID, itemID := createRunItemSnapshotFixture(t, database)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	results := make(chan runItemSnapshotResult, 1)
	go func() {
		items, err := readRunItemSnapshot(ctx, database, runID, itemID, direct)
		results <- runItemSnapshotResult{items: items, err: err}
	}()
	defer blocker.Resume()
	waitForRunItemScoreQuery(ctx, t, blocker)
	completeRunItemSnapshot(t, database, runID, itemID)
	blocker.Resume()
	assertRunItemSnapshotResult(ctx, t, results)
}

func waitForRunItemScoreQuery(
	ctx context.Context,
	t *testing.T,
	blocker *runItemScoreBlocker,
) {
	t.Helper()
	select {
	case <-blocker.started:
	case <-ctx.Done():
		t.Fatalf("wait for run item score query: %v", ctx.Err())
	}
}

func assertRunItemSnapshotResult(
	ctx context.Context,
	t *testing.T,
	results <-chan runItemSnapshotResult,
) {
	t.Helper()
	select {
	case result := <-results:
		if result.err != nil || len(result.items) != 1 ||
			result.items[0].Status != domain.EvalStatusRunning || len(result.items[0].Scores) != 0 {
			t.Fatalf("run item snapshot = %#v, %v", result.items, result.err)
		}
	case <-ctx.Done():
		t.Fatalf("read run item snapshot: %v", ctx.Err())
	}
}

type runItemSnapshotResult struct {
	items []domain.EvalRunItem
	err   error
}

func readRunItemSnapshot(
	ctx context.Context,
	database *Database,
	runID uuid.UUID,
	itemID uuid.UUID,
	direct bool,
) ([]domain.EvalRunItem, error) {
	if direct {
		item, err := database.GetEvalRunItem(ctx, runID, itemID)
		return []domain.EvalRunItem{item}, err
	}
	return database.ListEvalRunItems(ctx, runID, domain.PageQuery{Limit: 1})
}

type runItemScoreBlocker struct {
	started     chan struct{}
	resume      chan struct{}
	startedOnce sync.Once
	resumeOnce  sync.Once
}

func newRunItemScoreBlocker() *runItemScoreBlocker {
	return &runItemScoreBlocker{started: make(chan struct{}), resume: make(chan struct{})}
}

func (b *runItemScoreBlocker) TraceQueryStart(
	ctx context.Context,
	_ *pgx.Conn,
	data pgx.TraceQueryStartData,
) context.Context {
	if strings.Contains(data.SQL, "dataset_item_id = ANY") {
		b.startedOnce.Do(func() {
			close(b.started)
			select {
			case <-b.resume:
			case <-ctx.Done():
			}
		})
	}
	return ctx
}

func (*runItemScoreBlocker) TraceQueryEnd(
	context.Context,
	*pgx.Conn,
	pgx.TraceQueryEndData,
) {
}

func (b *runItemScoreBlocker) Resume() {
	b.resumeOnce.Do(func() { close(b.resume) })
}

func createRunItemSnapshotFixture(t *testing.T, database *Database) (uuid.UUID, uuid.UUID) {
	t.Helper()
	projectID := uuid.Must(uuid.NewV7())
	applicationID := uuid.Must(uuid.NewV7())
	datasetID := uuid.Must(uuid.NewV7())
	itemID := uuid.Must(uuid.NewV7())
	runID := uuid.Must(uuid.NewV7())
	mustExecSnapshot(t, database, "insert project", `
INSERT INTO projects (id, name) VALUES ($1, 'Run item snapshot')`, projectID)
	mustExecSnapshot(t, database, "insert application", `
INSERT INTO applications (id, project_id, name, slug)
VALUES ($1, $2, 'Run item snapshot', 'run-item-snapshot')`, applicationID, projectID)
	mustExecSnapshot(t, database, "insert dataset", `
INSERT INTO datasets (id, application_id, name) VALUES ($1, $2, 'Cases')`,
		datasetID, applicationID)
	mustExecSnapshot(t, database, "insert dataset item", `
INSERT INTO dataset_items (id, dataset_id, input, output)
VALUES ($1, $2, '{"question":"question"}', 'answer')`, itemID, datasetID)
	mustExecSnapshot(t, database, "insert eval run", `
INSERT INTO eval_runs (id, application_id, dataset_id, name, status, mode, scorers, total_items)
VALUES ($1, $2, $3, 'Run', 'running', 'score_existing', ARRAY['groundedness'], 1)`,
		runID, applicationID, datasetID)
	mustExecSnapshot(t, database, "insert eval run item", `
INSERT INTO eval_run_items (
    eval_run_id, dataset_item_id, status, snapshot_dataset_id, snapshot_input,
    snapshot_output, snapshot_context, snapshot_metadata, snapshot_created_at,
    snapshot_updated_at, snapshot_origin
) VALUES ($1, $2, 'running', $3, '{"question":"question"}', 'answer', '[]', '{}',
          now(), now(), 'creation')`, runID, itemID, datasetID)
	return runID, itemID
}

func completeRunItemSnapshot(
	t *testing.T,
	database *Database,
	runID uuid.UUID,
	itemID uuid.UUID,
) {
	t.Helper()
	transaction, err := database.pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin run item completion: %v", err)
	}
	defer func() { _ = transaction.Rollback(t.Context()) }()
	_, err = transaction.Exec(t.Context(), `
INSERT INTO scores (
    scorer, value, threshold, passed, rationale, prompt_template_id,
    judge_model, judge_provider, eval_run_id, dataset_item_id
) VALUES ('groundedness', 1, 0.5, true, 'supported', 'groundedness@v1',
          'judge', 'fake', $1, $2)`, runID, itemID)
	if err != nil {
		t.Fatalf("insert completed score: %v", err)
	}
	_, err = transaction.Exec(t.Context(), `
UPDATE eval_run_items SET status = 'succeeded', finished_at = now(), updated_at = now()
WHERE eval_run_id = $1 AND dataset_item_id = $2`, runID, itemID)
	if err != nil {
		t.Fatalf("complete run item: %v", err)
	}
	if err := transaction.Commit(t.Context()); err != nil {
		t.Fatalf("commit run item completion: %v", err)
	}
}

var _ pgx.QueryTracer = (*runItemScoreBlocker)(nil)
