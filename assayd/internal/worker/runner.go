// Package worker executes durable offline evaluation jobs.
package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/scoring"

	"github.com/google/uuid"
)

// RunRepository persists execution state and score results.
type RunRepository interface {
	StartEvalRun(context.Context, uuid.UUID, domain.JobLease) (domain.EvalRun, error)
	ListPendingEvalRunItems(context.Context, uuid.UUID) ([]domain.EvalRunItem, error)
	MarkEvalRunItemRunning(context.Context, uuid.UUID, uuid.UUID, domain.JobLease) error
	ResetEvalRunItemPending(context.Context, uuid.UUID, uuid.UUID, string, domain.JobLease) error
	FailEvalRunItem(context.Context, uuid.UUID, uuid.UUID, string, domain.JobLease) error
	CompleteEvalRunItem(
		context.Context, uuid.UUID, uuid.UUID, []domain.Score, domain.JobLease,
	) error
}

type jobRetryError struct {
	err error
}

func (e *jobRetryError) Error() string { return e.err.Error() }
func (e *jobRetryError) Unwrap() error { return e.err }

// ScorerConfigResolver provides executable scorer configurations.
type ScorerConfigResolver interface {
	ResolveScorerConfigs(
		context.Context,
		uuid.UUID,
		[]string,
		domain.JudgeDefaults,
	) ([]domain.ResolvedScorerConfig, error)
}

// Runner scores every pending item in one evaluation run.
type Runner struct {
	repository RunRepository
	resolver   ScorerConfigResolver
	registry   scoring.Registry
	client     *http.Client
	defaults   domain.JudgeDefaults
}

// NewRunner constructs an evaluation runner.
func NewRunner(
	repository RunRepository,
	resolver ScorerConfigResolver,
	client *http.Client,
	defaults domain.JudgeDefaults,
) *Runner {
	return &Runner{
		repository: repository, resolver: resolver, registry: scoring.NewRegistry(),
		client: client, defaults: defaults,
	}
}

// Run executes every pending item in an evaluation run.
func (r *Runner) Run(ctx context.Context, runID uuid.UUID, lease domain.JobLease) error {
	run, configByName, items, err := r.prepareRun(ctx, runID, lease)
	if err != nil {
		return err
	}
	return r.runItems(ctx, run, items, configByName, lease)
}

func (r *Runner) prepareRun(
	ctx context.Context,
	runID uuid.UUID,
	lease domain.JobLease,
) (domain.EvalRun, map[string]domain.ResolvedScorerConfig, []domain.EvalRunItem, error) {
	run, err := r.repository.StartEvalRun(ctx, runID, lease)
	if err != nil {
		return domain.EvalRun{}, nil, nil, fmt.Errorf("start evaluation run: %w", err)
	}
	configs, err := r.resolver.ResolveScorerConfigs(
		ctx, run.ApplicationID, run.Scorers, r.defaults,
	)
	if err != nil {
		return domain.EvalRun{}, nil, nil, fmt.Errorf("resolve scorer configurations: %w", err)
	}
	items, err := r.repository.ListPendingEvalRunItems(ctx, run.ID)
	if err != nil {
		return domain.EvalRun{}, nil, nil, fmt.Errorf("list pending run items: %w", err)
	}
	configByName := make(map[string]domain.ResolvedScorerConfig, len(configs))
	for _, config := range configs {
		configByName[config.Scorer] = config
	}
	return run, configByName, items, nil
}

func (r *Runner) runItems(
	ctx context.Context,
	run domain.EvalRun,
	items []domain.EvalRunItem,
	configs map[string]domain.ResolvedScorerConfig,
	lease domain.JobLease,
) error {
	var retryErr error
	for _, item := range items {
		itemErr := r.scoreItem(ctx, run, item, configs, lease)
		if itemErr == nil {
			continue
		}
		var jobErr *jobRetryError
		if errors.As(itemErr, &jobErr) {
			return itemErr
		}
		if retryableItemError(itemErr) {
			if resetErr := r.repository.ResetEvalRunItemPending(
				ctx, run.ID, item.DatasetItemID, itemErr.Error(), lease,
			); resetErr != nil {
				return fmt.Errorf("reset retryable run item: %w", resetErr)
			}
			if retryErr == nil {
				retryErr = itemErr
			}
			continue
		}
		if failErr := r.repository.FailEvalRunItem(
			ctx, run.ID, item.DatasetItemID, itemErr.Error(), lease,
		); failErr != nil {
			return fmt.Errorf("persist failed run item: %w", failErr)
		}
	}
	return retryErr
}

func retryableItemError(err error) bool {
	return scoring.IsRetryable(err) || errors.Is(err, context.Canceled)
}

func (r *Runner) scoreItem(
	ctx context.Context,
	run domain.EvalRun,
	item domain.EvalRunItem,
	configs map[string]domain.ResolvedScorerConfig,
	lease domain.JobLease,
) error {
	if err := r.repository.MarkEvalRunItemRunning(
		ctx, run.ID, item.DatasetItemID, lease,
	); err != nil {
		return &jobRetryError{err: fmt.Errorf("start run item: %w", err)}
	}
	input, err := scoreInput(item.Item)
	if err != nil {
		return err
	}
	scores, err := r.scoreResults(ctx, run, item.DatasetItemID, input, configs)
	if err != nil {
		return err
	}
	if err := r.repository.CompleteEvalRunItem(
		ctx, run.ID, item.DatasetItemID, scores, lease,
	); err != nil {
		return &jobRetryError{err: fmt.Errorf("complete run item: %w", err)}
	}
	return nil
}

func scoreInput(item domain.DatasetItem) (scoring.ScoreInput, error) {
	question, ok := item.Input["question"].(string)
	if !ok || question == "" || item.Output == nil {
		return scoring.ScoreInput{}, errors.New("dataset item requires string question and output")
	}
	reference := ""
	if item.ExpectedOutput != nil {
		reference = *item.ExpectedOutput
	}
	return scoring.ScoreInput{
		Input: question, Output: *item.Output, Context: item.Context, Reference: reference,
	}, nil
}

func (r *Runner) scoreResults(
	ctx context.Context,
	run domain.EvalRun,
	itemID uuid.UUID,
	input scoring.ScoreInput,
	configs map[string]domain.ResolvedScorerConfig,
) ([]domain.Score, error) {
	scores := make([]domain.Score, 0, len(run.Scorers))
	for _, name := range run.Scorers {
		scorer, found := r.registry.Get(name)
		config, configured := configs[name]
		if !found || !configured {
			return nil, fmt.Errorf("scorer %q is not configured", name)
		}
		result, err := scorer.Score(ctx, input, scoring.NewHTTPJudge(r.client, config.Judge))
		if err != nil {
			return nil, fmt.Errorf("score %s: %w", name, err)
		}
		scores = append(scores, scoreRecord(run.ID, itemID, config, result))
	}
	return scores, nil
}

func scoreRecord(
	runID uuid.UUID,
	itemID uuid.UUID,
	config domain.ResolvedScorerConfig,
	result scoring.ScoreResult,
) domain.Score {
	return domain.Score{
		Scorer: config.Scorer, ScorerConfigID: config.ConfigID,
		Value: result.Value, Threshold: config.Threshold, Passed: result.Value >= config.Threshold,
		Rationale: result.Rationale, Details: result.Details,
		PromptTemplateID: config.PromptTemplateID, JudgeModel: config.Judge.Model,
		JudgeProvider: config.Judge.Provider, JudgeTokens: result.JudgeTokens,
		EvalRunID: runID, DatasetItemID: itemID,
	}
}
