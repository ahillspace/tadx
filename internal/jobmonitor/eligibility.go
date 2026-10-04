package jobmonitor

import (
	"context"
	"sync"
)

// Eligibility remembers one command's optional monitoring capability per target.
// A failed role read selects the synchronous publication contract.
type Eligibility struct {
	mu    sync.Mutex
	known map[string]bool
}

func (e *Eligibility) CanMonitor(ctx context.Context, kind, key string, canInspect func() bool, probe func(context.Context) (bool, error)) (bool, error) {
	if kind == "flow" || !canInspect() {
		return false, nil
	}
	e.mu.Lock()
	known, present := e.known[key]
	e.mu.Unlock()
	if present {
		return known, nil
	}
	known, err := probe(ctx)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err != nil {
		known = false
	}
	e.mu.Lock()
	if e.known == nil {
		e.known = make(map[string]bool)
	}
	e.known[key] = known
	e.mu.Unlock()
	return known, nil
}
