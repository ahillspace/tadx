package app

import (
	"context"
	"time"
)

// publicationExecution belongs to one invocation, not one batch item.
// Its deadline ends observation only; the worker retains ownership of writes.
type publicationExecution struct {
	operation string
	noWait    bool
	deadline  time.Time
	accepted  func(context.Context, string) error
	detached  func() bool
}

func (r *runtimeDependencies) publicationWaitDeadline() time.Time {
	if r.publicationExecution != nil {
		return r.publicationExecution.deadline
	}
	return time.Time{}
}

func (r *runtimeDependencies) publicationNoWait() bool {
	e := r.publicationExecution
	return e != nil && (e.noWait || (e.detached != nil && e.detached()) || (!e.deadline.IsZero() && !time.Now().Before(e.deadline)))
}
