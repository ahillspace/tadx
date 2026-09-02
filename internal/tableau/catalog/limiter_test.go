package catalog

import (
	"context"
	"testing"
	"time"
)

func TestAdaptiveLimiterEnforcesAndChangesConcurrency(t *testing.T) {
	limiter := newAdaptiveLimiter(1, 4)
	if err := limiter.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	acquired := make(chan struct{})
	go func() {
		if limiter.Acquire(context.Background()) == nil {
			close(acquired)
		}
	}()
	select {
	case <-acquired:
		t.Fatal("second request passed a limit of one")
	case <-time.After(20 * time.Millisecond):
	}
	limiter.Release(true, false)
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("second request remained blocked after the limit increased")
	}
	limiter.Release(true, false)
	if limiter.Limit() < 2 {
		t.Fatalf("success did not increase limit: %d", limiter.Limit())
	}
	if err := limiter.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	limiter.Release(false, true)
	if limiter.Limit() != 1 {
		t.Fatalf("throttle did not reduce limit: %d", limiter.Limit())
	}
}

func TestAdaptiveLimiterWaitHonorsCancellation(t *testing.T) {
	limiter := newAdaptiveLimiter(1, 1)
	if err := limiter.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := limiter.Acquire(ctx); err != context.Canceled {
		t.Fatalf("error = %v", err)
	}
	limiter.Release(false, false)
}
