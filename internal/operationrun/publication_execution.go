package operationrun

import (
	"context"
	"time"
)

// PublicationExecution belongs to one invocation, not one batch item.
// Its deadline ends observation only; the worker retains ownership of writes.
type PublicationExecution struct {
	Operation   string
	OperationID string
	NoWait      bool
	Deadline    time.Time
	Detached    func() bool
	Prepare     func(context.Context, string, string, string, string) error
	Accepted    func(context.Context, string) error
}

func (e *PublicationExecution) WaitDeadline() time.Time {
	if e == nil {
		return time.Time{}
	}
	return e.Deadline
}

func (e *PublicationExecution) StopRequested() bool {
	return e != nil && (e.NoWait || (e.Detached != nil && e.Detached()) || (!e.Deadline.IsZero() && !time.Now().Before(e.Deadline)))
}

func (e *PublicationExecution) HasPrepareIntent() bool { return e != nil && e.Prepare != nil }

func (e *PublicationExecution) RecordIntent(ctx context.Context, operation, receiptID, scope string) error {
	if !e.HasPrepareIntent() {
		return nil
	}
	return e.Prepare(ctx, e.OperationID, operation, receiptID, scope)
}

func (e *PublicationExecution) LinkAccepted(ctx context.Context, path string) error {
	if e == nil || e.Accepted == nil {
		return nil
	}
	return e.Accepted(ctx, path)
}
