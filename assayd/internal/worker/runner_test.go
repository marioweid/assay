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
	"github.com/marioweid/assay/assayd/internal/target"

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
	runner := testRunner(repository, resolver, judge.Client())

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
	runner := testRunner(repository, resolver, judge.Client())
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
	runner := testRunner(repository, resolver, judge.Client())

	err := runner.Run(t.Context(), runID, testLease())
	var retryErr *jobRetryError
	if !errors.As(err, &retryErr) || repository.failed {
		t.Fatalf("run error/failed = %v/%v, want job retry without item failure", err, repository.failed)
	}
}

//nolint:cyclop // The two snapshot states share one behavioral assertion sequence.
func TestRunnerPersistsAndReusesGenerationSnapshot(t *testing.T) {
	judge := httptest.NewServer(http.HandlerFunc(runnerJudgeHandler(t)))
	t.Cleanup(judge.Close)
	for _, existing := range []bool{false, true} {
		t.Run("existing="+strconv.FormatBool(existing), func(t *testing.T) {
			runID := uuid.Must(uuid.NewV7())
			itemID := uuid.Must(uuid.NewV7())
			item := domain.EvalRunItem{
				EvalRunID: runID, DatasetItemID: itemID,
				Item: domain.DatasetItem{
					ID: itemID, Input: map[string]any{"question": "What is Assay?"},
					Context: []domain.Chunk{{ID: "fallback", Text: "fallback context"}},
				},
			}
			if existing {
				output := "Assay evaluates AI."
				item.GeneratedOutput = &output
				item.GeneratedContext = []domain.Chunk{{ID: "saved", Text: "saved context"}}
			}
			repository := &runnerRepositoryFake{
				run: domain.EvalRun{
					ID: runID, ApplicationID: uuid.Must(uuid.NewV7()),
					Mode:    domain.EvalModeGenerateThenScore,
					Scorers: []string{domain.ScorerGroundedness},
				},
				items: []domain.EvalRunItem{item},
			}
			generator := &generatorFake{generation: domain.Generation{
				Output:  "Assay evaluates AI.",
				Context: []domain.Chunk{{ID: "k0", Text: "Assay evaluates AI."}},
			}}
			runner := NewRunner(RunnerDependencies{
				Repository: repository,
				Resolver: scorerResolverFake{config: domain.ResolvedScorerConfig{
					Scorer: domain.ScorerGroundedness, Threshold: 0.5,
					PromptTemplateID: domain.GroundednessPromptV1,
					Judge:            domain.ResolvedJudgeConfig{BaseURL: judge.URL, Model: "fake"},
				}},
				Targets: targetResolverFake{}, Generator: generator, JudgeClient: judge.Client(),
			})

			if err := runner.Run(t.Context(), runID, testLease()); err != nil {
				t.Fatalf("run generated evaluation: %v", err)
			}
			wantCalls := 1
			if existing {
				wantCalls = 0
			}
			if generator.calls != wantCalls {
				t.Fatalf("target calls = %d, want %d", generator.calls, wantCalls)
			}
			if !existing && (repository.saved.Output != "Assay evaluates AI." ||
				len(repository.events) < 2 || repository.events[0] != "generation") {
				t.Fatalf("saved generation/events = %#v/%#v", repository.saved, repository.events)
			}
		})
	}
}

func testRunner(
	repository RunRepository,
	resolver ScorerConfigResolver,
	client *http.Client,
) *Runner {
	return NewRunner(RunnerDependencies{
		Repository: repository, Resolver: resolver, JudgeClient: client,
	})
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

type targetResolverFake struct{}

func (targetResolverFake) ResolveTargetEndpoint(
	context.Context,
	uuid.UUID,
) (domain.ResolvedTargetEndpoint, error) {
	return domain.ResolvedTargetEndpoint{
		ResponseMapping: domain.ResponseMapping{Output: "$.answer"},
	}, nil
}

type generatorFake struct {
	generation domain.Generation
	calls      int
}

func (f *generatorFake) Compile(domain.ResponseMapping) (target.Mapping, error) {
	return target.Mapping{}, nil
}

func (f *generatorFake) Generate(
	context.Context,
	domain.ResolvedTargetEndpoint,
	target.Mapping,
	domain.DatasetItem,
) (domain.Generation, error) {
	f.calls++
	return f.generation, nil
}

type runnerRepositoryFake struct {
	run         domain.EvalRun
	items       []domain.EvalRunItem
	scores      []domain.Score
	completeErr error
	failed      bool
	reset       bool
	saved       domain.Generation
	events      []string
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
	f.events = append(f.events, "complete")
	f.scores = scores
	return f.completeErr
}

func (f *runnerRepositoryFake) SaveEvalRunItemGeneration(
	_ context.Context,
	_ uuid.UUID,
	_ uuid.UUID,
	generation domain.Generation,
	_ domain.JobLease,
) error {
	f.saved = generation
	f.events = append(f.events, "generation")
	return nil
}

func testLease() domain.JobLease {
	return domain.JobLease{JobID: uuid.Must(uuid.NewV7()), WorkerID: "worker"}
}
