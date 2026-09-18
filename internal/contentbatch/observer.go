package contentbatch

import "context"

type observerKey struct{}

// WithObserver reports local item transitions without polling remote work.
// Observers must copy any result they retain beyond the call.
func WithObserver(ctx context.Context, observer func(Output)) context.Context {
	return context.WithValue(ctx, observerKey{}, observer)
}

func notify(ctx context.Context, out Output) {
	if observer, ok := ctx.Value(observerKey{}).(func(Output)); ok && observer != nil {
		observer(out)
	}
}
