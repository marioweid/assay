package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/store"

	"github.com/google/uuid"
)

func TestTraceDeletionRemovesTelemetryAndScoringEvidence(t *testing.T) {
	database := openTraceDatabase(t)
	service, traceService := newTraceServices(t, database)
	project, application := createTraceApplication(t, service, "Lifecycle")
	trace := traceFixture(application.ID)
	if err := traceService.Ingest(t.Context(), project.ID, []domain.Trace{trace}); err != nil {
		t.Fatalf("ingest trace: %v", err)
	}
	detail, err := traceService.Get(t.Context(), project.ID, trace.ID)
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if _, err := database.QueueTraceScores(
		t.Context(), project.ID,
		[]domain.TraceScoreRequest{{
			TraceID: detail.ID, Scorer: domain.ScorerGroundedness, JobID: uuid.Must(uuid.NewV7()),
			MaxAttempts: 3,
		}},
		false,
	); err != nil {
		t.Fatalf("queue trace scores: %v", err)
	}

	if err := database.DeleteTrace(t.Context(), detail.ID); err != nil {
		t.Fatalf("delete trace: %v", err)
	}
	if _, err := traceService.GetAdmin(t.Context(), detail.ID); !errors.Is(
		err, domain.ErrNotFound,
	) {
		t.Fatalf("get deleted trace error = %v, want ErrNotFound", err)
	}
	var count int
	if err := database.MigrationDB().QueryRowContext(
		t.Context(),
		`SELECT count(*) FROM spans WHERE trace_id = $1`,
		detail.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count deleted spans: %v", err)
	}
	if count != 0 {
		t.Fatalf("deleted trace spans remaining = %d", count)
	}
}

func TestTraceDeletionDoesNotBlockUnrelatedHeartbeat(t *testing.T) {
	database := openTraceDatabase(t)
	service, traces := newTraceServices(t, database)
	project, application := createTraceApplication(t, service, "Scoped Delete")
	related := traceFixture(application.ID)
	unrelated := traceFixture(application.ID)
	unrelated.OTelTraceID[15] = 4
	if err := traces.Ingest(t.Context(), project.ID, []domain.Trace{related, unrelated}); err != nil {
		t.Fatalf("ingest deletion fixtures: %v", err)
	}
	relatedJob := queueDeletionFixtureJob(t, database, related.ID)
	unrelatedJob := queueDeletionFixtureJob(t, database, unrelated.ID)
	markJobRunning(t, database, unrelatedJob.ID, "unrelated-worker")

	blocker, err := database.MigrationDB().BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin related job blocker: %v", err)
	}
	deletionFinished := make(chan error, 1)
	deletionStarted := false
	defer func() {
		_ = blocker.Rollback()
		if deletionStarted {
			<-deletionFinished
		}
	}()
	if _, err := blocker.ExecContext(
		t.Context(), "SELECT id FROM jobs WHERE id = $1 FOR UPDATE", relatedJob.ID,
	); err != nil {
		t.Fatalf("lock related job: %v", err)
	}
	go func() { deletionFinished <- database.DeleteTrace(t.Context(), related.ID) }()
	deletionStarted = true
	waitForBlockedJobQuery(t, database)

	heartbeatContext, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := database.HeartbeatJob(
		heartbeatContext, unrelatedJob.ID, "unrelated-worker", 30*time.Second,
	); err != nil {
		t.Fatalf("heartbeat unrelated job during trace deletion: %v", err)
	}
	if err := blocker.Rollback(); err != nil {
		t.Fatalf("release related job blocker: %v", err)
	}
	if err := <-deletionFinished; err != nil {
		t.Fatalf("delete trace after blocker release: %v", err)
	}
	deletionStarted = false
}

func queueDeletionFixtureJob(
	t *testing.T,
	database *store.Database,
	traceID uuid.UUID,
) domain.Job {
	t.Helper()
	job, err := database.QueueScoringTask(t.Context(), domain.Job{
		ID: uuid.Must(uuid.NewV7()), Kind: domain.JobKindScoringTask,
		TraceID: &traceID, Scorer: domain.ScorerGroundedness, MaxAttempts: 3,
	}, false)
	if err != nil {
		t.Fatalf("queue deletion fixture job: %v", err)
	}
	return job
}

func markJobRunning(
	t *testing.T,
	database *store.Database,
	jobID uuid.UUID,
	worker string,
) {
	t.Helper()
	if _, err := database.MigrationDB().ExecContext(
		t.Context(),
		`UPDATE jobs SET status = 'running', locked_by = $2, locked_at = now(),
		 lease_expires_at = now() + interval '1 minute' WHERE id = $1`,
		jobID,
		worker,
	); err != nil {
		t.Fatalf("mark unrelated job running: %v", err)
	}
}

func waitForBlockedJobQuery(t *testing.T, database *store.Database) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		err := database.MigrationDB().QueryRowContext(t.Context(), `
			SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity
				WHERE datname = current_database() AND state = 'active'
				  AND wait_event_type = 'Lock' AND query LIKE '%jobs%'
			)`).Scan(&blocked)
		if err != nil {
			t.Fatalf("inspect blocked deletion: %v", err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("trace deletion did not block on its related job")
}

func TestEvalRunDeletionRefusesActiveRuns(t *testing.T) {
	database, _, runID, _ := createLeaseFixture(t)
	job, err := database.ClaimJob(t.Context(), "worker-a", 30*time.Second)
	if err != nil {
		t.Fatalf("claim fixture job: %v", err)
	}
	if _, err := database.StartEvalRun(t.Context(), runID, domain.JobLease{
		JobID: job.ID, WorkerID: "worker-a",
	}); err != nil {
		t.Fatalf("start eval run: %v", err)
	}

	if err := database.DeleteEvalRun(t.Context(), runID); !errors.Is(
		err, domain.ErrConflict,
	) {
		t.Fatalf("delete active run error = %v, want ErrConflict", err)
	}
	if _, err := database.GetEvalRun(t.Context(), runID); err != nil {
		t.Fatalf("active run was removed: %v", err)
	}
}

func TestEvalRunDeletionRemovesTerminalRunEvidence(t *testing.T) {
	database, evaluations, runID, _ := createLeaseFixture(t)
	if _, err := database.MigrationDB().ExecContext(
		t.Context(),
		`UPDATE eval_runs SET status = 'succeeded', finished_at = now() WHERE id = $1`,
		runID,
	); err != nil {
		t.Fatalf("finish fixture run: %v", err)
	}
	if err := evaluations.DeleteEvalRun(t.Context(), runID); err != nil {
		t.Fatalf("delete terminal run: %v", err)
	}
	if _, err := evaluations.GetEvalRun(t.Context(), runID); !errors.Is(
		err, domain.ErrNotFound,
	) {
		t.Fatalf("get deleted run error = %v, want ErrNotFound", err)
	}
}
