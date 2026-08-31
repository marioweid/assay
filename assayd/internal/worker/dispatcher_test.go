package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestDispatcherRoutesTypedJobs(t *testing.T) {
	eval := &evalJobRunnerFake{}
	trace := &traceJobRunnerFake{}
	dispatcher := NewDispatcher(eval, trace)
	runID := uuid.Must(uuid.NewV7())
	traceID := uuid.Must(uuid.NewV7())
	lease := testLease()
	if err := dispatcher.Run(t.Context(), domain.Job{
		Kind: domain.JobKindEvalRun, EvalRunID: &runID,
	}, lease); err != nil {
		t.Fatalf("dispatch eval job: %v", err)
	}
	job := domain.Job{Kind: domain.JobKindScoringTask, TraceID: &traceID, Scorer: "groundedness"}
	if err := dispatcher.Run(t.Context(), job, lease); err != nil {
		t.Fatalf("dispatch scoring task: %v", err)
	}
	if eval.runID != runID || trace.job.TraceID == nil || *trace.job.TraceID != traceID {
		t.Fatalf("dispatched eval/trace = %s/%#v", eval.runID, trace.job)
	}
	if err := dispatcher.Run(t.Context(), domain.Job{Kind: "invalid"}, lease); !errors.Is(
		err, domain.ErrInvalid,
	) {
		t.Fatalf("invalid dispatch error = %v", err)
	}
}

type evalJobRunnerFake struct{ runID uuid.UUID }

func (f *evalJobRunnerFake) Run(
	_ context.Context,
	runID uuid.UUID,
	_ domain.JobLease,
) error {
	f.runID = runID
	return nil
}

type traceJobRunnerFake struct{ job domain.Job }

func (f *traceJobRunnerFake) Run(
	_ context.Context,
	job domain.Job,
	_ domain.JobLease,
) error {
	f.job = job
	return nil
}
