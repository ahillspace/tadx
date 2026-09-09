package create

import "context"

// A resolver may share project reads within one explicit validation phase.
// Each prewrite phase starts again, never inheriting the planning snapshot.
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
