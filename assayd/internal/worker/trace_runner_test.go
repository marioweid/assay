package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

//nolint:cyclop // Assertions cover the complete persisted online score contract.
func TestTraceRunnerPersistsOnlineScoreAuditSnapshot(t *testing.T) {
	judge := httptest.NewServer(http.HandlerFunc(runnerJudgeHandler(t)))
	t.Cleanup(judge.Close)
	traceID := uuid.Must(uuid.NewV7())
	applicationID := uuid.Must(uuid.NewV7())
	trace := scoringTraceForRunner(traceID, applicationID)
	repository := &traceWorkRepositoryFake{trace: trace}
	resolver := scorerResolverFake{config: domain.ResolvedScorerConfig{
		Scorer: domain.ScorerGroundedness, Threshold: 0.7,
		PromptTemplateID: domain.GroundednessPromptV1,
		Judge: domain.ResolvedJudgeConfig{
			BaseURL: judge.URL, Model: "fake", Provider: "fake",
		},
	}}
	runner := NewTraceRunner(repository, resolver, judge.Client(), domain.JudgeDefaults{})

	jobID := uuid.Must(uuid.NewV7())
	err := runner.Run(t.Context(), domain.Job{
		ID: jobID, Kind: domain.JobKindScoringTask, TraceID: &traceID,
		Scorer: domain.ScorerGroundedness,
	}, domain.JobLease{JobID: jobID, WorkerID: "worker"})
	if err != nil {
		t.Fatalf("run trace score: %v", err)
	}
	score := repository.score
	if score.TraceID == nil || *score.TraceID != traceID || score.SpanID == nil ||
		score.JudgedInput != "What is Assay?" || score.JudgedOutput != "Assay evaluates AI." ||
		len(score.JudgedContext) != 1 || score.Value != 1 || !score.Passed {
		t.Fatalf("online score = %#v", score)
	}
}

func scoringTraceForRunner(traceID uuid.UUID, applicationID uuid.UUID) domain.Trace {
	return domain.Trace{
		ID: traceID, ApplicationID: applicationID,
		Spans: []domain.Span{{
			ID: 42, TraceID: traceID, ApplicationID: applicationID, IsScorable: true,
			Attributes: map[string]any{
				"gen_ai.input.messages": []any{
					map[string]any{"role": "user", "content": "What is Assay?"},
				},
				"gen_ai.output.messages": []any{
					map[string]any{"role": "assistant", "content": "Assay evaluates AI."},
				},
				"gen_ai.retrieval.documents": []any{
					map[string]any{"id": "k0", "text": "Assay evaluates AI."},
				},
			},
		}},
	}
}

type traceWorkRepositoryFake struct {
	trace domain.Trace
	score domain.Score
}

func (f *traceWorkRepositoryFake) GetTraceForScoring(
	context.Context,
	uuid.UUID,
) (domain.Trace, error) {
	return f.trace, nil
}

func (f *traceWorkRepositoryFake) CompleteTraceScore(
	_ context.Context,
	score domain.Score,
	_ domain.JobLease,
) error {
	f.score = score
	return nil
}
