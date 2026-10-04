package app

import (
	"context"

	mutation "github.com/ahillspace/tadx/actions/mutation"
	session "github.com/ahillspace/tadx/actions/session"
	"github.com/ahillspace/tadx/internal/config"
)

func newSessionService(runtime *runtimeDependencies) *session.Service {
	return session.New(session.Dependencies{
		ReadConfiguration: runtime.configuration,
		SiteSetting:       mutation.SiteSetting,
		LookupEnv:         processEnvironment{},
		ResolveWorkspace: func(ctx context.Context, cfg config.Config, selector, environmentDefault string) (session.WorkspaceResolution, error) {
			selected, err := runtime.resolveWorkspace(ctx, cfg, selector, environmentDefault)
			return session.WorkspaceResolution{Name: selected.Name, Reason: selected.SelectionReason}, err
		},
	})
}
