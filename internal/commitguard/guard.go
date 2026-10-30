// Package commitguard carries operation-specific admission to irreversible boundaries.
package commitguard

import "context"

type key struct{}

type denied struct{ cause error }

func (e denied) Error() string { return e.cause.Error() }
func (e denied) Unwrap() error { return e.cause }

// NotAttempted reports that the guarded transaction was never admitted.
func (denied) NotAttempted() bool { return true }

// WithAdmission associates a commit admission callback with an operation context.
func WithAdmission(ctx context.Context, admit func(context.Context) error) context.Context {
	return context.WithValue(ctx, key{}, admit)
}

// Admit checks cancellation and admission before starting a transaction.
// An admitted transaction must complete even if its operation is canceled.
func Admit(ctx context.Context) (context.Context, error) {
	admit, ok := ctx.Value(key{}).(func(context.Context) error)
	if !ok || admit == nil {
		return ctx, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, denied{cause: err}
	}
	if err := admit(ctx); err != nil {
		return nil, denied{cause: err}
	}
	return context.WithoutCancel(ctx), nil
}
