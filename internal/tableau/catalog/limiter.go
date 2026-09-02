package catalog

import (
	"context"
	"sync"
)

type adaptiveLimiter struct {
	mu        sync.Mutex
	limit     int
	maximum   int
	inFlight  int
	successes int
	notify    chan struct{}
}

func newAdaptiveLimiter(initial, maximum int) *adaptiveLimiter {
	return &adaptiveLimiter{limit: initial, maximum: maximum, notify: make(chan struct{})}
}

func (l *adaptiveLimiter) Acquire(ctx context.Context) error {
	for {
		l.mu.Lock()
		if l.inFlight < l.limit {
			l.inFlight++
			l.mu.Unlock()
			return nil
		}
		notify := l.notify
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-notify:
		}
	}
}

func (l *adaptiveLimiter) Release(success, throttled bool) {
	l.mu.Lock()
	l.inFlight--
	if throttled {
		l.successes = 0
		l.limit = max(1, l.limit/2)
	} else if success {
		l.successes++
		if l.successes >= l.limit && l.limit < l.maximum {
			l.successes = 0
			l.limit++
		}
	}
	close(l.notify)
	l.notify = make(chan struct{})
	l.mu.Unlock()
}

func (l *adaptiveLimiter) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}
