package publish

import "context"

// A retained Apply and its post-upload checks each require a fresh project view.
type projectResolutionPhase interface {
	BeginProjectResolution(context.Context) context.Context
}

func (a *Action) beginProjectResolution(ctx context.Context) context.Context {
	if a != nil {
		if resolver, ok := a.resolver.(projectResolutionPhase); ok {
			return resolver.BeginProjectResolution(ctx)
		}
	}
	return ctx
}
