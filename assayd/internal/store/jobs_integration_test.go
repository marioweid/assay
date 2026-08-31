package store_test

import (
	"errors"
	"testing"
	"time"

	secretcrypto "github.com/marioweid/assay/assayd/internal/crypto"
	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/store"

	"github.com/google/uuid"
)

func TestJobLeaseReapingOwnershipAndReleaseExhaustion(t *testing.T) {
	database, evaluations, runID, itemID := createLeaseFixture(t)
	job := claimJob(t, database, "worker-a", 500*time.Millisecond)
	leaseA := domain.JobLease{JobID: job.ID, WorkerID: "worker-a"}
	startRunItem(t, database, runID, itemID, leaseA)
	time.Sleep(600 * time.Millisecond)
	assertStaleHeartbeat(t, database, job.ID)
	assertNoClaimableJob(t, database)
	if err := database.ReapExpiredJobs(t.Context()); err != nil {
		t.Fatalf("reap expired job: %v", err)
	}
	assertStaleRunMutation(t, database, runID, itemID, leaseA)

	job = claimJob(t, database, "worker-b", 30*time.Second)
	assertDatabaseLease(t, job)
	leaseB := domain.JobLease{JobID: job.ID, WorkerID: "worker-b"}
	if _, err := database.StartEvalRun(t.Context(), runID, leaseB); err != nil {
		t.Fatalf("restart eval run: %v", err)
	}
	assertRunItemPending(t, evaluations, runID)
	if err := database.RetryJob(t.Context(), job.ID, "worker-b", 0, "retry"); err != nil {
		t.Fatalf("retry job: %v", err)
	}
	job = claimJob(t, database, "worker-c", 30*time.Second)
	leaseC := domain.JobLease{JobID: job.ID, WorkerID: "worker-c"}
	startRunItem(t, database, runID, itemID, leaseC)
	if err := database.ReleaseWorkerJobs(t.Context(), "worker-c"); err != nil {
		t.Fatalf("release exhausted worker job: %v", err)
	}
	run := assertRunExhausted(t, evaluations, runID)
	if _, err := evaluations.CancelEvalRun(t.Context(), runID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("cancel terminal run error = %v, want ErrConflict", err)
	}
	if err := evaluations.DeleteDataset(t.Context(), run.DatasetID); err != nil {
		t.Fatalf("delete dataset with terminal job: %v", err)
	}
	if _, err := evaluations.GetEvalRun(t.Context(), runID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted run error = %v, want ErrNotFound", err)
	}
}

//nolint:cyclop // The integration test checks each durable transition in one lifecycle.
func TestScoringTaskClaimsAndCompletesWithoutEvalRun(t *testing.T) {
	database := openTraceDatabase(t)
	service, traces := newTraceServices(t, database)
	project, application := createTraceApplication(t, service, "Online Queue")
	trace := traceFixture(application.ID)
	if err := traces.Ingest(t.Context(), project.ID, []domain.Trace{trace}); err != nil {
		t.Fatalf("ingest trace: %v", err)
	}
	page, err := traces.List(t.Context(), project.ID, domain.TraceQuery{Limit: 1})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("list trace: %#v, %v", page.Items, err)
	}
	traceID := page.Items[0].ID
	task, err := database.QueueScoringTask(t.Context(), domain.Job{
		ID: uuid.Must(uuid.NewV7()), Kind: domain.JobKindScoringTask,
		TraceID: &traceID, Scorer: domain.ScorerGroundedness, MaxAttempts: 3,
	}, false)
	if err != nil {
		t.Fatalf("queue scoring task: %v", err)
	}
	claimed := claimJob(t, database, "online-worker", 30*time.Second)
	if claimed.ID != task.ID || claimed.TraceID == nil || *claimed.TraceID != traceID ||
		claimed.EvalRunID != nil || claimed.Scorer != domain.ScorerGroundedness {
		t.Fatalf("claimed scoring task = %#v", claimed)
	}
	if err := database.CompleteJob(t.Context(), claimed.ID, "online-worker"); err != nil {
		t.Fatalf("complete scoring task: %v", err)
	}
}

func createLeaseFixture(
	t *testing.T,
) (*store.Database, *domain.EvaluationService, uuid.UUID, uuid.UUID) {
	t.Helper()
	database := openTraceDatabase(t)
	service, _ := newTraceServices(t, database)
	_, application := createTraceApplication(t, service, "Queue")
	cipher, err := secretcrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	evaluations := domain.NewEvaluationService(database, cipher, 3)
	dataset, err := evaluations.CreateDataset(t.Context(), domain.CreateDatasetInput{
		ApplicationID: application.ID, Name: "queue-cases",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	items, err := evaluations.CreateDatasetItems(
		t.Context(), dataset.ID,
		[]domain.CreateDatasetItemInput{{
			Input: map[string]any{"question": "question"}, Output: "answer",
		}},
	)
	if err != nil {
		t.Fatalf("create dataset item: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("created dataset items = %#v", items)
	}
	run, err := evaluations.CreateEvalRun(t.Context(), domain.CreateEvalRunInput{
		ApplicationID: application.ID, DatasetID: dataset.ID, Name: "queue-run",
		Scorers: []string{domain.ScorerGroundedness},
	})
	if err != nil {
		t.Fatalf("create eval run: %v", err)
	}
	return database, evaluations, run.ID, items[0].ID
}

func assertNoClaimableJob(t *testing.T, database *store.Database) {
	t.Helper()
	_, err := database.ClaimJob(t.Context(), "worker-b", 30*time.Second)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second claim error = %v, want ErrNotFound", err)
	}
}

func claimJob(
	t *testing.T,
	database *store.Database,
	workerID string,
	leaseDuration time.Duration,
) domain.Job {
	t.Helper()
	job, err := database.ClaimJob(t.Context(), workerID, leaseDuration)
	if err != nil {
		t.Fatalf("claim job for %s: %v", workerID, err)
	}
	return job
}

func startRunItem(
	t *testing.T,
	database *store.Database,
	runID uuid.UUID,
	itemID uuid.UUID,
	lease domain.JobLease,
) {
	t.Helper()
	if _, err := database.StartEvalRun(t.Context(), runID, lease); err != nil {
		t.Fatalf("start eval run: %v", err)
	}
	if err := database.MarkEvalRunItemRunning(
		t.Context(), runID, itemID, lease,
	); err != nil {
		t.Fatalf("start eval run item: %v", err)
	}
}

func assertStaleHeartbeat(t *testing.T, database *store.Database, jobID uuid.UUID) {
	t.Helper()
	if err := database.HeartbeatJob(
		t.Context(), jobID, "worker-a", 30*time.Second,
	); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("stale heartbeat error = %v, want ErrNotFound", err)
	}
}

func assertStaleRunMutation(
	t *testing.T,
	database *store.Database,
	runID uuid.UUID,
	itemID uuid.UUID,
	lease domain.JobLease,
) {
	t.Helper()
	err := database.MarkEvalRunItemRunning(t.Context(), runID, itemID, lease)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("stale run mutation error = %v, want ErrNotFound", err)
	}
}

func assertDatabaseLease(t *testing.T, job domain.Job) {
	t.Helper()
	if job.LeaseExpiresAt == nil || job.LockedAt == nil ||
		job.LeaseExpiresAt.Sub(*job.LockedAt) < 29*time.Second {
		t.Fatalf("database lease timestamps = %#v", job)
	}
}

func assertRunItemPending(
	t *testing.T,
	evaluations *domain.EvaluationService,
	runID uuid.UUID,
) {
	t.Helper()
	page, err := evaluations.ListEvalRunItems(
		t.Context(), runID, domain.PageQuery{Limit: 10},
	)
	if err != nil {
		t.Fatalf("list reaped run items: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Status != domain.EvalStatusPending {
		t.Fatalf("reaped run items = %#v", page.Items)
	}
}

func assertRunExhausted(
	t *testing.T,
	evaluations *domain.EvaluationService,
	runID uuid.UUID,
) domain.EvalRun {
	t.Helper()
	run, err := evaluations.GetEvalRun(t.Context(), runID)
	if err != nil {
		t.Fatalf("get exhausted run: %v", err)
	}
	if run.Status != domain.EvalStatusFailed || run.FailedItems != 1 {
		t.Fatalf("exhausted run = %#v", run)
	}
	return run
}
