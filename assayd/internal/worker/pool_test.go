package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestHeartbeatFailureCancelsExecution(t *testing.T) {
	repository := heartbeatRepositoryFake{}
	pool := NewPool(
		repository, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), "worker", 1,
	)
	pool.heartbeatEvery = time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	var leaseLost atomic.Bool
	pool.heartbeat(ctx, "worker", uuid.Must(uuid.NewV7()), heartbeatControl{
		done: make(chan struct{}), cancel: cancel, leaseLost: &leaseLost,
	})
	if !leaseLost.Load() || ctx.Err() == nil {
		t.Fatalf("lease lost/canceled = %v/%v", leaseLost.Load(), ctx.Err())
	}
}

type heartbeatRepositoryFake struct{}

func (heartbeatRepositoryFake) ClaimJob(
	context.Context, string, time.Duration,
) (domain.Job, error) {
	return domain.Job{}, domain.ErrNotFound
}

func (heartbeatRepositoryFake) HeartbeatJob(
	context.Context, uuid.UUID, string, time.Duration,
) error {
	return errors.New("lease lost")
}

func (heartbeatRepositoryFake) CompleteJob(context.Context, uuid.UUID, string) error { return nil }

func (heartbeatRepositoryFake) RetryJob(
	context.Context, uuid.UUID, string, time.Duration, string,
) error {
	return nil
}

func (heartbeatRepositoryFake) ExhaustJob(context.Context, uuid.UUID, string, string) error {
	return nil
}

func (heartbeatRepositoryFake) ReapExpiredJobs(context.Context) error { return nil }

func (heartbeatRepositoryFake) ReleaseWorkerJobs(context.Context, string) error { return nil }
