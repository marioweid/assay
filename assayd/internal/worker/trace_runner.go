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

// TraceWorkRepository loads score inputs and atomically completes online scores.
type TraceWorkRepository interface {
	GetTraceForScoring(context.Context, uuid.UUID) (domain.Trace, error)
	CompleteTraceScore(context.Context, domain.Score, domain.JobLease) error
}

// TraceRunner executes one online trace scoring task.
type TraceRunner struct {
	repository TraceWorkRepository
	resolver   ScorerConfigResolver
	registry   scoring.Registry
	client     *http.Client
	defaults   domain.JudgeDefaults
}

// NewTraceRunner constructs an online trace scorer.
func NewTraceRunner(
	repository TraceWorkRepository,
	resolver ScorerConfigResolver,
	client *http.Client,
	defaults domain.JudgeDefaults,
) *TraceRunner {
	return &TraceRunner{
		repository: repository, resolver: resolver, registry: scoring.NewRegistry(),
		client: client, defaults: defaults,
	}
}

// Run scores and persists the trace targeted by one typed job.
//
//nolint:cyclop // The worker classifies each external and permanent failure before persistence.
func (r *TraceRunner) Run(ctx context.Context, job domain.Job, lease domain.JobLease) error {
	if job.Kind != domain.JobKindScoringTask || job.TraceID == nil || job.Scorer == "" {
		return fmt.Errorf("run trace score: %w: invalid job target", domain.ErrInvalid)
	}
	trace, err := r.repository.GetTraceForScoring(ctx, *job.TraceID)
	if err != nil {
		return &jobRetryError{err: fmt.Errorf("load trace for scoring: %w", err)}
	}
	input, err := domain.BuildTraceScoreInput(trace, job.Scorer)
	if err != nil {
		return err
	}
	configs, err := r.resolver.ResolveScorerConfigs(
		ctx, trace.ApplicationID, []string{job.Scorer}, r.defaults,
	)
	if err != nil {
		return fmt.Errorf("resolve trace scorer: %w", err)
	}
	if len(configs) != 1 {
		return fmt.Errorf("resolve trace scorer: %w: expected one config", domain.ErrInvalid)
	}
	scorer, found := r.registry.Get(job.Scorer)
	if !found {
		return fmt.Errorf("run trace score: %w: unsupported scorer", domain.ErrInvalid)
	}
	result, err := scorer.Score(ctx, scoring.ScoreInput{
		Input: input.Input, Output: input.Output, Context: input.Context,
		Reference: input.Reference,
	}, scoring.NewHTTPJudge(r.client, configs[0].Judge))
	if err != nil {
		if scoring.IsRetryable(err) || errors.Is(err, context.Canceled) {
			return fmt.Errorf("score trace with %s: %w", job.Scorer, err)
		}
		return fmt.Errorf("score trace with %s: %w", job.Scorer, errors.Join(domain.ErrInvalid, err))
	}
	score := onlineScore(trace, input, configs[0], result)
	if err := r.repository.CompleteTraceScore(ctx, score, lease); err != nil {
		return &jobRetryError{err: fmt.Errorf("complete trace score: %w", err)}
	}
	return nil
}

func onlineScore(
	trace domain.Trace,
	input domain.TraceScoreInput,
	config domain.ResolvedScorerConfig,
	result scoring.ScoreResult,
) domain.Score {
	traceID, spanID, spanStart := trace.ID, input.Span.ID, input.Span.StartTime
	score := domain.Score{
		Scorer: config.Scorer, ScorerConfigID: config.ConfigID,
		Value: result.Value, Threshold: config.Threshold, Passed: result.Value >= config.Threshold,
		Rationale: result.Rationale, Details: result.Details,
		PromptTemplateID: config.PromptTemplateID, JudgeModel: config.Judge.Model,
		JudgeProvider: config.Judge.Provider, JudgeTokens: result.JudgeTokens,
		TraceID: &traceID, SpanID: &spanID, SpanStartTime: &spanStart,
		JudgedInput: input.Input, JudgedOutput: input.Output, JudgedContext: input.Context,
	}
	if input.Reference != "" {
		reference := input.Reference
		score.JudgedReference = &reference
	}
	return score
}
