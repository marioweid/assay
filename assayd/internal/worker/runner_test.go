package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/scoring"

	"github.com/google/uuid"
)

func TestRunnerScoresItem(t *testing.T) {
	judge := httptest.NewServer(http.HandlerFunc(runnerJudgeHandler(t)))
	t.Cleanup(judge.Close)

	runID := uuid.Must(uuid.NewV7())
	itemID := uuid.Must(uuid.NewV7())
	output := "Assay evaluates AI."
	repository := &runnerRepositoryFake{
		run: domain.EvalRun{
			ID: runID, ApplicationID: uuid.Must(uuid.NewV7()),
			Status: domain.EvalStatusPending, Scorers: []string{domain.ScorerGroundedness},
		},
		items: []domain.EvalRunItem{{
			EvalRunID: runID, DatasetItemID: itemID,
			Item: domain.DatasetItem{
				ID: itemID, Input: map[string]any{"question": "What is Assay?"}, Output: &output,
				Context: []domain.Chunk{{ID: "k0", Text: "Assay evaluates AI."}},
			},
		}},
	}
	resolver := scorerResolverFake{config: domain.ResolvedScorerConfig{
		Scorer: domain.ScorerGroundedness, Threshold: 0.7,
		PromptTemplateID: domain.GroundednessPromptV1,
		Judge: domain.ResolvedJudgeConfig{
			BaseURL: judge.URL, Model: "fake", Provider: "fake",
		},
	}}
	runner := NewRunner(repository, resolver, judge.Client(), domain.JudgeDefaults{})

	if err := runner.Run(t.Context(), runID, testLease()); err != nil {
		t.Fatalf("run evaluation: %v", err)
	}
	if len(repository.scores) != 1 {
		t.Fatalf("scores = %#v", repository.scores)
	}
	if repository.scores[0].Value != 1 || !repository.scores[0].Passed ||
		repository.scores[0].JudgeTokens != 8 {
		t.Fatalf("score = %#v", repository.scores[0])
	}
}

func TestRunnerContinuesSiblingsAfterRetryableJudgeFailure(t *testing.T) {
	calls := 0
	judge := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		calls++
		if calls == 1 {
			http.Error(writer, "unavailable", http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(
			`{"choices":[{"message":{"content":"{\"claims\":[],\"excluded\":[]}"}}]}`,
		))
	}))
	t.Cleanup(judge.Close)
	runID := uuid.Must(uuid.NewV7())
	itemID := uuid.Must(uuid.NewV7())
	siblingID := uuid.Must(uuid.NewV7())
	output := "answer"
	repository := &runnerRepositoryFake{
		run: domain.EvalRun{
			ID: runID, ApplicationID: uuid.Must(uuid.NewV7()),
			Scorers: []string{domain.ScorerGroundedness},
		},
		items: []domain.EvalRunItem{{
			EvalRunID: runID, DatasetItemID: itemID,
			Item: domain.DatasetItem{
				ID: itemID, Input: map[string]any{"question": "question"}, Output: &output,
			},
		}, {
			EvalRunID: runID, DatasetItemID: siblingID,
			Item: domain.DatasetItem{
				ID: siblingID, Input: map[string]any{"question": "sibling"}, Output: &output,
			},
		}},
	}
	resolver := scorerResolverFake{config: domain.ResolvedScorerConfig{
		Scorer: domain.ScorerGroundedness, Threshold: 0.5,
		PromptTemplateID: domain.GroundednessPromptV1,
		Judge:            domain.ResolvedJudgeConfig{BaseURL: judge.URL, Model: "fake"},
	}}
	runner := NewRunner(repository, resolver, judge.Client(), domain.JudgeDefaults{})
	err := runner.Run(t.Context(), runID, testLease())
	if !scoring.IsRetryable(err) {
		t.Fatalf("run error = %v, want retryable", err)
	}
	if !repository.reset || len(repository.scores) != 1 {
		t.Fatalf(
			"reset/scores = %v/%#v",
			repository.reset, repository.scores,
		)
	}
}

func TestRunnerRetriesJobAfterScorePersistenceFailure(t *testing.T) {
	judge := httptest.NewServer(http.HandlerFunc(runnerJudgeHandler(t)))
	t.Cleanup(judge.Close)
	runID := uuid.Must(uuid.NewV7())
	itemID := uuid.Must(uuid.NewV7())
	output := "Assay evaluates AI."
	repository := &runnerRepositoryFake{
		run: domain.EvalRun{
			ID: runID, ApplicationID: uuid.Must(uuid.NewV7()),
			Status: domain.EvalStatusPending, Scorers: []string{domain.ScorerGroundedness},
		},
		items: []domain.EvalRunItem{{
			EvalRunID: runID, DatasetItemID: itemID,
			Item: domain.DatasetItem{
				ID: itemID, Input: map[string]any{"question": "What is Assay?"}, Output: &output,
				Context: []domain.Chunk{{ID: "k0", Text: output}},
			},
		}},
		completeErr: errors.New("database unavailable"),
	}
	resolver := scorerResolverFake{config: domain.ResolvedScorerConfig{
		Scorer: domain.ScorerGroundedness, Threshold: 0.7,
		PromptTemplateID: domain.GroundednessPromptV1,
		Judge:            domain.ResolvedJudgeConfig{BaseURL: judge.URL, Model: "fake"},
	}}
	runner := NewRunner(repository, resolver, judge.Client(), domain.JudgeDefaults{})

	err := runner.Run(t.Context(), runID, testLease())
	var retryErr *jobRetryError
	if !errors.As(err, &retryErr) || repository.failed {
		t.Fatalf("run error/failed = %v/%v, want job retry without item failure", err, repository.failed)
	}
}

func runnerJudgeHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode judge request: %v", err)
		}
		content := `{"claims":[{"id":"c1","text":"Assay evaluates AI."}],"excluded":[]}`
		if len(body.Messages) > 0 && strings.HasPrefix(body.Messages[0].Content, "Judge") {
			content = `{"verdicts":[{"id":"c1","verdict":"supported",` +
				`"supporting_chunk_ids":["k0"],"reason":"Direct support."}]}`
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":` +
			strconv.Quote(content) + `}}],"usage":{"total_tokens":4}}`))
	}
}

type scorerResolverFake struct{ config domain.ResolvedScorerConfig }

func (f scorerResolverFake) ResolveScorerConfigs(
	_ context.Context,
	_ uuid.UUID,
	_ []string,
	_ domain.JudgeDefaults,
) ([]domain.ResolvedScorerConfig, error) {
	return []domain.ResolvedScorerConfig{f.config}, nil
}

type runnerRepositoryFake struct {
	run         domain.EvalRun
	items       []domain.EvalRunItem
	scores      []domain.Score
	completeErr error
	failed      bool
	reset       bool
}

func (f *runnerRepositoryFake) StartEvalRun(
	_ context.Context,
	_ uuid.UUID,
	_ domain.JobLease,
) (domain.EvalRun, error) {
	f.run.Status = domain.EvalStatusRunning
	return f.run, nil
}

func (f *runnerRepositoryFake) ListPendingEvalRunItems(
	_ context.Context,
	_ uuid.UUID,
) ([]domain.EvalRunItem, error) {
	return f.items, nil
}

func (f *runnerRepositoryFake) MarkEvalRunItemRunning(
	_ context.Context,
	_ uuid.UUID,
	_ uuid.UUID,
	_ domain.JobLease,
) error {
	return nil
}

func (f *runnerRepositoryFake) ResetEvalRunItemPending(
	_ context.Context,
	_ uuid.UUID,
	_ uuid.UUID,
	_ string,
	_ domain.JobLease,
) error {
	f.reset = true
	return nil
}

func (f *runnerRepositoryFake) FailEvalRunItem(
	_ context.Context,
	_ uuid.UUID,
	_ uuid.UUID,
	_ string,
	_ domain.JobLease,
) error {
	f.failed = true
	return nil
}

func (f *runnerRepositoryFake) CompleteEvalRunItem(
	_ context.Context,
	_ uuid.UUID,
	_ uuid.UUID,
	scores []domain.Score,
	_ domain.JobLease,
) error {
	f.scores = scores
	return f.completeErr
}

func testLease() domain.JobLease {
	return domain.JobLease{JobID: uuid.Must(uuid.NewV7()), WorkerID: "worker"}
}
