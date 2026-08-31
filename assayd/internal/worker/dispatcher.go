package worker

import (
	"context"
	"fmt"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

// EvalJobRunner executes an offline evaluation run target.
type EvalJobRunner interface {
	Run(context.Context, uuid.UUID, domain.JobLease) error
}

// TraceJobRunner executes an online trace scoring target.
type TraceJobRunner interface {
	Run(context.Context, domain.Job, domain.JobLease) error
}

// Dispatcher routes durable jobs by their typed target.
type Dispatcher struct {
	eval  EvalJobRunner
	trace TraceJobRunner
}

// NewDispatcher constructs a typed durable-job dispatcher.
func NewDispatcher(eval EvalJobRunner, trace TraceJobRunner) *Dispatcher {
	return &Dispatcher{eval: eval, trace: trace}
}

// Run validates and dispatches one job.
//
//nolint:cyclop // Two typed variants require explicit target-shape guards.
func (d *Dispatcher) Run(ctx context.Context, job domain.Job, lease domain.JobLease) error {
	switch job.Kind {
	case domain.JobKindEvalRun:
		if job.EvalRunID == nil || job.TraceID != nil {
			return fmt.Errorf("dispatch eval run: %w: invalid target", domain.ErrInvalid)
		}
		return d.eval.Run(ctx, *job.EvalRunID, lease)
	case domain.JobKindScoringTask:
		if job.TraceID == nil || job.EvalRunID != nil || job.Scorer == "" {
			return fmt.Errorf("dispatch trace score: %w: invalid target", domain.ErrInvalid)
		}
		return d.trace.Run(ctx, job, lease)
	default:
		return fmt.Errorf("dispatch job: %w: unsupported kind %q", domain.ErrInvalid, job.Kind)
	}
}
