package cache

import (
	"context"
	"sync"
	"time"
)

type adaptiveLimiter struct {
	mu            sync.Mutex
	limit         int
	maximum       int
	inFlight      int
	successes     int
	notify        chan struct{}
	cooldownUntil time.Time
}

func newAdaptiveLimiter(initial, maximum int) *adaptiveLimiter {
	return &adaptiveLimiter{limit: initial, maximum: maximum, notify: make(chan struct{})}
}

func (l *adaptiveLimiter) Acquire(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		l.mu.Lock()
		delay := time.Until(l.cooldownUntil)
		if l.inFlight < l.limit && delay <= 0 {
			l.inFlight++
			l.mu.Unlock()
			return nil
		}
		notify := l.notify
		l.mu.Unlock()
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-notify:
				timer.Stop()
			case <-timer.C:
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-notify:
		}
	}
}

func (l *adaptiveLimiter) Release(success, throttled bool) {
	l.release(success, throttled, 0)
}

// Publish the cooldown before releasing the slot so queued workers cannot
// escape a server delay between the response and the capacity notification.
func (l *adaptiveLimiter) release(success, throttled bool, cooldown time.Duration) {
	l.mu.Lock()
	l.inFlight--
	if cooldown > 0 {
		until := time.Now().Add(cooldown)
		if until.After(l.cooldownUntil) {
			l.cooldownUntil = until
		}
	}
	if throttled {
		l.successes = 0
		l.limit = max(1, l.limit/2)
	} else if success && !time.Now().Before(l.cooldownUntil) {
		l.successes++
		if l.successes >= l.limit && l.limit < l.maximum {
			l.successes = 0
			l.limit++
		}
	}
	// Wake waiters only when a slot is actually available. When a throttle just
	// halved the limit the freed in-flight slot may still leave inFlight >=
	// limit, so broadcasting would wake every waiter for a guaranteed re-block
	// (a thundering herd at high concurrency). Any transition into available
	// capacity happens here and closes the current notify channel, so a parked
	// waiter is never stranded.
	if l.inFlight < l.limit {
		close(l.notify)
		l.notify = make(chan struct{})
	}
	l.mu.Unlock()
}

func (l *adaptiveLimiter) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}
