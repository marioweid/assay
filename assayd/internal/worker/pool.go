package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

const (
	pollInterval      = 250 * time.Millisecond
	leaseDuration     = 30 * time.Second
	heartbeatInterval = 10 * time.Second
	reaperInterval    = 10 * time.Second
	drainTimeout      = 30 * time.Second
)

// JobRepository persists worker leases and terminal job state.
type JobRepository interface {
	ClaimJob(context.Context, string, time.Duration) (domain.Job, error)
	HeartbeatJob(context.Context, uuid.UUID, string, time.Duration) error
	CompleteJob(context.Context, uuid.UUID, string) error
	RetryJob(context.Context, uuid.UUID, string, time.Duration, string) error
	ExhaustJob(context.Context, uuid.UUID, string, string) error
	ReapExpiredJobs(context.Context) error
	ReleaseWorkerJobs(context.Context, string) error
}

// JobRunner executes one typed durable job.
type JobRunner interface {
	Run(context.Context, domain.Job, domain.JobLease) error
}

// Pool owns concurrent durable-job consumers.
type Pool struct {
	repository     JobRepository
	runner         JobRunner
	logger         *slog.Logger
	workerID       string
	concurrency    int
	heartbeatEvery time.Duration
}

type heartbeatControl struct {
	done      <-chan struct{}
	cancel    context.CancelFunc
	leaseLost *atomic.Bool
}

// NewPool constructs a durable worker pool.
func NewPool(
	repository JobRepository,
	runner JobRunner,
	logger *slog.Logger,
	workerID string,
	concurrency int,
) *Pool {
	return &Pool{
		repository: repository, runner: runner, logger: logger,
		workerID: workerID, concurrency: concurrency, heartbeatEvery: heartbeatInterval,
	}
}

// Run consumes jobs until the context is canceled.
func (p *Pool) Run(ctx context.Context) {
	if err := p.repository.ReapExpiredJobs(ctx); err != nil {
		p.logger.Error("failed to reap expired jobs", "error", err)
	}
	claimCtx, stopClaims := context.WithCancel(ctx)
	defer stopClaims()
	workCtx, stopWork := context.WithCancel(context.WithoutCancel(ctx))
	defer stopWork()
	var workers sync.WaitGroup
	workers.Add(p.concurrency + 1)
	go func() {
		defer workers.Done()
		p.reaperLoop(claimCtx)
	}()
	for index := range p.concurrency {
		go func() {
			defer workers.Done()
			p.workerLoop(claimCtx, workCtx, fmt.Sprintf("%s-%d", p.workerID, index))
		}()
	}
	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-ctx.Done():
	}
	timer := time.NewTimer(drainTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		stopWork()
		<-done
	}
}

func (p *Pool) reaperLoop(ctx context.Context) {
	ticker := time.NewTicker(reaperInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.repository.ReapExpiredJobs(ctx); err != nil {
				p.logger.Error("failed to reap expired jobs", "error", err)
			}
		}
	}
}

func (p *Pool) workerLoop(claimCtx context.Context, workCtx context.Context, workerID string) {
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.repository.ReleaseWorkerJobs(cleanup, workerID); err != nil {
			p.logger.Error("failed to release worker jobs", "worker_id", workerID, "error", err)
		}
	}()
	for claimCtx.Err() == nil {
		job, err := p.repository.ClaimJob(claimCtx, workerID, leaseDuration)
		if errors.Is(err, domain.ErrNotFound) {
			if !waitForPoll(claimCtx) {
				return
			}
			continue
		}
		if err != nil {
			p.logger.Error("failed to claim job", "worker_id", workerID, "error", err)
			if !waitForPoll(claimCtx) {
				return
			}
			continue
		}
		p.executeJob(workCtx, workerID, job)
	}
}

func (p *Pool) executeJob(ctx context.Context, workerID string, job domain.Job) {
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	var heartbeat sync.WaitGroup
	var leaseLost atomic.Bool
	heartbeat.Add(1)
	go func() {
		defer heartbeat.Done()
		p.heartbeat(jobCtx, workerID, job.ID, heartbeatControl{
			done: done, cancel: cancel, leaseLost: &leaseLost,
		})
	}()
	lease := domain.JobLease{JobID: job.ID, WorkerID: workerID}
	err := p.runner.Run(jobCtx, job, lease)
	if !leaseLost.Load() && ctx.Err() == nil {
		p.persistJobResult(jobCtx, workerID, job, err)
	}
	close(done)
	heartbeat.Wait()
}

func (p *Pool) persistJobResult(
	ctx context.Context,
	workerID string,
	job domain.Job,
	jobErr error,
) {
	for ctx.Err() == nil {
		err := p.applyJobResult(ctx, workerID, job, jobErr)
		if err == nil || errors.Is(err, domain.ErrNotFound) {
			return
		}
		p.logger.Error("failed to persist job result", "job_id", job.ID, "error", err)
		if !waitForPoll(ctx) {
			return
		}
	}
}

func (p *Pool) applyJobResult(
	ctx context.Context,
	workerID string,
	job domain.Job,
	jobErr error,
) error {
	if jobErr == nil {
		return p.repository.CompleteJob(ctx, job.ID, workerID)
	}
	if shouldRetry(job, jobErr) {
		return p.repository.RetryJob(
			ctx, job.ID, workerID, retryDelay(job.Attempts), jobErr.Error(),
		)
	}
	return p.repository.ExhaustJob(ctx, job.ID, workerID, jobErr.Error())
}

func shouldRetry(job domain.Job, err error) bool {
	return !errors.Is(err, domain.ErrInvalid) && job.Attempts < job.MaxAttempts
}

func (p *Pool) heartbeat(
	ctx context.Context,
	workerID string,
	jobID uuid.UUID,
	control heartbeatControl,
) {
	ticker := time.NewTicker(p.heartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-control.done:
			return
		case <-ticker.C:
			if err := p.repository.HeartbeatJob(ctx, jobID, workerID, leaseDuration); err != nil {
				p.logger.Error("failed to heartbeat job", "job_id", jobID, "error", err)
				control.leaseLost.Store(true)
				control.cancel()
				return
			}
		}
	}
}

func waitForPoll(ctx context.Context) bool {
	timer := time.NewTimer(pollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func retryDelay(attempt int) time.Duration {
	shift := max(attempt-1, 0)
	if shift >= 6 {
		return time.Minute
	}
	delay := time.Second << shift
	return min(delay, time.Minute)
}
