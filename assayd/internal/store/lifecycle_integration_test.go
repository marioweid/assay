package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

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
