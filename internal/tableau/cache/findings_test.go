package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestProtocolErrorTaxonomyMatchesEngineBehavior locks the corrected taxonomy:
// post-fetch protocol errors are never retried by the engine and are not HTTP
// failures, so Retryable is false and HTTPStatus is 0 rather than a misleading
// 200.
func TestProtocolErrorTaxonomyMatchesEngineBehavior(t *testing.T) {
	err := newProtocolError("cache.projects.list", "req-1", errors.New("garbled page"))
	if err.Retryable() {
		t.Fatal("protocolError advertised retryability but the engine never retries it")
	}
	if isRetryable(err) {
		t.Fatal("isRetryable classified a protocol error as retryable")
	}
	if got := err.HTTPStatus(); got != 0 {
		t.Fatalf("HTTPStatus = %d; want 0 for a non-HTTP failure", got)
	}
	if got := statusCode(err); got != 0 {
		t.Fatalf("statusCode = %d; want 0 for a non-HTTP failure", got)
	}
	if err.RequestID() != "req-1" {
		t.Fatalf("RequestID = %q; want req-1", err.RequestID())
	}
}

// TestAdaptiveLimiterThrottleDoesNotWakeWaiterWithoutCapacity verifies the
// broadcast-suppression fix: a throttle that frees an in-flight slot but leaves
// inFlight >= limit must not wake a waiter (no capacity), yet a later release
// that genuinely frees a slot must.
func TestAdaptiveLimiterThrottleDoesNotWakeWaiterWithoutCapacity(t *testing.T) {
	limiter := newAdaptiveLimiter(2, 4)
	if err := limiter.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
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
		t.Fatal("third request passed a limit of two")
	case <-time.After(20 * time.Millisecond):
	}

	// Throttle halves the limit (2 -> 1) while freeing one slot (inFlight 2 ->
	// 1). Capacity is still full, so the waiter must stay blocked.
	limiter.Release(false, true)
	if limiter.Limit() != 1 {
		t.Fatalf("throttle did not reduce limit: %d", limiter.Limit())
	}
	select {
	case <-acquired:
		t.Fatal("waiter woke while capacity was still full after throttle")
	case <-time.After(20 * time.Millisecond):
	}

	// A successful release frees the last slot and grows the limit; the waiter
	// must now proceed.
	limiter.Release(true, false)
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("waiter remained blocked after a slot actually freed")
	}
	limiter.Release(true, false)
}
