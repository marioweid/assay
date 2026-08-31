// Package worker executes durable offline evaluation jobs.
package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/scoring"
	"github.com/marioweid/assay/assayd/internal/target"

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
	SaveEvalRunItemGeneration(
		context.Context, uuid.UUID, uuid.UUID, domain.Generation, domain.JobLease,
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

// TargetResolver provides executable application endpoint settings.
type TargetResolver interface {
	ResolveTargetEndpoint(context.Context, uuid.UUID) (domain.ResolvedTargetEndpoint, error)
}

// Generator compiles mappings and invokes target endpoints.
type Generator interface {
	Compile(domain.ResponseMapping) (target.Mapping, error)
	Generate(
		context.Context,
		domain.ResolvedTargetEndpoint,
		target.Mapping,
		domain.DatasetItem,
	) (domain.Generation, error)
}

// RunnerDependencies contains the collaborators used by an evaluation runner.
type RunnerDependencies struct {
	Repository  RunRepository
	Resolver    ScorerConfigResolver
	Targets     TargetResolver
	Generator   Generator
	JudgeClient *http.Client
	Defaults    domain.JudgeDefaults
}

// Runner scores every pending item in one evaluation run.
type Runner struct {
	repository RunRepository
	resolver   ScorerConfigResolver
	registry   scoring.Registry
	client     *http.Client
	defaults   domain.JudgeDefaults
	targets    TargetResolver
	generator  Generator
}

// NewRunner constructs an evaluation runner.
func NewRunner(dependencies RunnerDependencies) *Runner {
	return &Runner{
		repository: dependencies.Repository, resolver: dependencies.Resolver,
		registry: scoring.NewRegistry(), client: dependencies.JudgeClient,
		defaults: dependencies.Defaults, targets: dependencies.Targets,
		generator: dependencies.Generator,
	}
}

type preparedRun struct {
	run      domain.EvalRun
	configs  map[string]domain.ResolvedScorerConfig
	items    []domain.EvalRunItem
	endpoint *domain.ResolvedTargetEndpoint
	mapping  target.Mapping
}

// Run executes every pending item in an evaluation run.
func (r *Runner) Run(ctx context.Context, runID uuid.UUID, lease domain.JobLease) error {
	prepared, err := r.prepareRun(ctx, runID, lease)
	if err != nil {
		return err
	}
	return r.runItems(ctx, prepared, lease)
}

//nolint:cyclop // Preparation validates durable state and optional target dependencies in one pass.
func (r *Runner) prepareRun(
	ctx context.Context,
	runID uuid.UUID,
	lease domain.JobLease,
) (preparedRun, error) {
	run, err := r.repository.StartEvalRun(ctx, runID, lease)
	if err != nil {
		return preparedRun{}, fmt.Errorf("start evaluation run: %w", err)
	}
	configs, err := r.resolver.ResolveScorerConfigs(
		ctx, run.ApplicationID, run.Scorers, r.defaults,
	)
	if err != nil {
		return preparedRun{}, fmt.Errorf("resolve scorer configurations: %w", err)
	}
	items, err := r.repository.ListPendingEvalRunItems(ctx, run.ID)
	if err != nil {
		return preparedRun{}, fmt.Errorf("list pending run items: %w", err)
	}
	configByName := make(map[string]domain.ResolvedScorerConfig, len(configs))
	for _, config := range configs {
		configByName[config.Scorer] = config
	}
	prepared := preparedRun{run: run, configs: configByName, items: items}
	if run.Mode != domain.EvalModeGenerateThenScore {
		return prepared, nil
	}
	if r.targets == nil || r.generator == nil {
		return preparedRun{}, errors.New("prepare generated run: target dependencies are unavailable")
	}
	endpoint, err := r.targets.ResolveTargetEndpoint(ctx, run.ApplicationID)
	if err != nil {
		return preparedRun{}, fmt.Errorf("resolve target endpoint: %w", err)
	}
	mapping, err := r.generator.Compile(endpoint.ResponseMapping)
	if err != nil {
		return preparedRun{}, fmt.Errorf(
			"compile target response mapping: %w", errors.Join(domain.ErrInvalid, err),
		)
	}
	prepared.endpoint = &endpoint
	prepared.mapping = mapping
	return prepared, nil
}

func (r *Runner) runItems(
	ctx context.Context,
	prepared preparedRun,
	lease domain.JobLease,
) error {
	var retryErr error
	for _, item := range prepared.items {
		itemErr := r.scoreItem(ctx, prepared, item, lease)
		if itemErr == nil {
			continue
		}
		var jobErr *jobRetryError
		if errors.As(itemErr, &jobErr) {
			return itemErr
		}
		if retryableItemError(itemErr) {
			if resetErr := r.repository.ResetEvalRunItemPending(
				ctx, prepared.run.ID, item.DatasetItemID, itemErr.Error(), lease,
			); resetErr != nil {
				return fmt.Errorf("reset retryable run item: %w", resetErr)
			}
			if retryErr == nil {
				retryErr = itemErr
			}
			continue
		}
		if failErr := r.repository.FailEvalRunItem(
			ctx, prepared.run.ID, item.DatasetItemID, itemErr.Error(), lease,
		); failErr != nil {
			return fmt.Errorf("persist failed run item: %w", failErr)
		}
	}
	return retryErr
}

func retryableItemError(err error) bool {
	return scoring.IsRetryable(err) || target.IsRetryable(err) || errors.Is(err, context.Canceled)
}

func (r *Runner) scoreItem(
	ctx context.Context,
	prepared preparedRun,
	item domain.EvalRunItem,
	lease domain.JobLease,
) error {
	if err := r.repository.MarkEvalRunItemRunning(
		ctx, prepared.run.ID, item.DatasetItemID, lease,
	); err != nil {
		return &jobRetryError{err: fmt.Errorf("start run item: %w", err)}
	}
	input, err := r.prepareScoreInput(ctx, prepared, item, lease)
	if err != nil {
		return err
	}
	scores, err := r.scoreResults(
		ctx, prepared.run, item.DatasetItemID, input, prepared.configs,
	)
	if err != nil {
		return err
	}
	if err := r.repository.CompleteEvalRunItem(
		ctx, prepared.run.ID, item.DatasetItemID, scores, lease,
	); err != nil {
		return &jobRetryError{err: fmt.Errorf("complete run item: %w", err)}
	}
	return nil
}

func (r *Runner) prepareScoreInput(
	ctx context.Context,
	prepared preparedRun,
	item domain.EvalRunItem,
	lease domain.JobLease,
) (scoring.ScoreInput, error) {
	if prepared.run.Mode != domain.EvalModeGenerateThenScore {
		return scoreInput(item.Item, item.Item.Output, item.Item.Context)
	}
	generation := domain.Generation{Context: item.GeneratedContext}
	if item.GeneratedOutput != nil {
		generation.Output = *item.GeneratedOutput
		return scoreInput(item.Item, &generation.Output, generation.Context)
	}
	generation, err := r.generator.Generate(ctx, *prepared.endpoint, prepared.mapping, item.Item)
	if err != nil {
		return scoring.ScoreInput{}, fmt.Errorf("generate target output: %w", err)
	}
	if err := r.repository.SaveEvalRunItemGeneration(
		ctx, prepared.run.ID, item.DatasetItemID, generation, lease,
	); err != nil {
		return scoring.ScoreInput{}, &jobRetryError{
			err: fmt.Errorf("save generated run item: %w", err),
		}
	}
	return scoreInput(item.Item, &generation.Output, generation.Context)
}

func scoreInput(
	item domain.DatasetItem,
	output *string,
	context []domain.Chunk,
) (scoring.ScoreInput, error) {
	question, ok := item.Input["question"].(string)
	if !ok || question == "" || output == nil {
		return scoring.ScoreInput{}, errors.New("dataset item requires string question and output")
	}
	reference := ""
	if item.ExpectedOutput != nil {
		reference = *item.ExpectedOutput
	}
	return scoring.ScoreInput{
		Input: question, Output: *output, Context: context, Reference: reference,
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
		EvalRunID: &runID, DatasetItemID: &itemID,
	}
}
