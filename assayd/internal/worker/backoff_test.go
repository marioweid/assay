package worker

import (
	"errors"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
)

func TestRetryDelayDoublesAndCaps(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: time.Second},
		{attempt: 2, want: 2 * time.Second},
		{attempt: 3, want: 4 * time.Second},
		{attempt: 20, want: time.Minute},
		{attempt: 36, want: time.Minute},
		{attempt: 1 << 30, want: time.Minute},
	}
	for _, test := range tests {
		if got := retryDelay(test.attempt); got != test.want {
			t.Errorf("retryDelay(%d) = %s, want %s", test.attempt, got, test.want)
		}
	}
}

func TestShouldRetryInfrastructureErrorsOnlyBeforeExhaustion(t *testing.T) {
	job := domain.Job{Attempts: 1, MaxAttempts: 3}
	if !shouldRetry(job, errors.New("database unavailable")) {
		t.Fatal("infrastructure error was not retryable")
	}
	if shouldRetry(job, domain.ErrInvalid) {
		t.Fatal("validation error was retryable")
	}
	job.Attempts = job.MaxAttempts
	if shouldRetry(job, errors.New("database unavailable")) {
		t.Fatal("exhausted job was retryable")
	}
}
