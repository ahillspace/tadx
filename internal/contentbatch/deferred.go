package contentbatch

import "context"

type completionKey struct{}
type completion struct {
	finish func(context.Context) (any, error)
}

// Bulk reports the resolved item-count rule, not a caller-controlled mode.
func Bulk(ctx context.Context) bool { _, ok := ctx.Value(completionKey{}).(*completion); return ok }

// DeferCompletion lets an accepted asynchronous operation finish after the
// bounded batch's submissions. It never delays submission or retries a write.
// A single item completes directly and must not use this hook.
func DeferCompletion(ctx context.Context, finish func(context.Context) (any, error)) bool {
	state, ok := ctx.Value(completionKey{}).(*completion)
	if !ok || state.finish != nil || finish == nil {
		return false
	}
	state.finish = finish
	return true
}
