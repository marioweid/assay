package domain_test

import (
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestJobTargetsRepresentEvalRunsAndScoringTasks(t *testing.T) {
	runID := uuid.Must(uuid.NewV7())
	traceID := uuid.Must(uuid.NewV7())
	evalJob := domain.Job{Kind: domain.JobKindEvalRun, EvalRunID: &runID}
	scoreJob := domain.Job{
		Kind: domain.JobKindScoringTask, TraceID: &traceID, Scorer: domain.ScorerGroundedness,
	}

	if evalJob.EvalRunID == nil || *evalJob.EvalRunID != runID || evalJob.TraceID != nil {
		t.Fatalf("eval job target = %#v", evalJob)
	}
	if scoreJob.TraceID == nil || *scoreJob.TraceID != traceID || scoreJob.EvalRunID != nil {
		t.Fatalf("scoring job target = %#v", scoreJob)
	}
}

func TestGenerationKeepsOutputAndContextTogether(t *testing.T) {
	generation := domain.Generation{
		Output:  "answer",
		Context: []domain.Chunk{{ID: "k0", Text: "context"}},
	}

	if generation.Output != "answer" || len(generation.Context) != 1 {
		t.Fatalf("generation = %#v", generation)
	}
}
