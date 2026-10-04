package app

import (
	"context"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	workspacecli "github.com/ahillspace/tadx/internal/cli/workspace"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func newWorkspaceCommands(runtime *runtimeDependencies) *workspacecli.Dependencies {
	selector := &workspaceRuntime{runtime: runtime}
	service := workspaceaction.NewLocalService(func() string { return runtime.configPath }, runtime.userHomeDir, selector.resolveForEnvironment)
	ids := []string{"workspace.create", "workspace.register", "workspace.clone", "workspace.list", "workspace.status", "workspace.set-default", "workspace.unregister", "workspace.delete", "workspace.move", "workspace.artifact.delete", "workspace.clean"}
	return &workspacecli.Dependencies{Creator: service, Registrar: service, Cloner: service, Lister: service, Statuser: service, DefaultSetter: service, Unregistrar: service, WorkspaceDeleter: service, Mover: service, Deleter: service, Cleaner: service, Uses: registryUses(ids...), Shorts: registryShorts(ids...)}
}

// workspaceRuntime binds command-scoped configuration and workspace selection.
type workspaceRuntime struct{ runtime *runtimeDependencies }

func (w *workspaceRuntime) resolveForEnvironment(ctx context.Context, selector, environmentAlias string) (workspacecore.Record, error) {
	configuration, err := w.runtime.configuration()
	if err != nil {
		return workspacecore.Record{}, err
	}
	environmentDefault := ""
	if environmentAlias != "" || configuration.DefaultEnvironment != "" {
		environment, resolveErr := configuration.ResolveEnvironment(environmentAlias)
		if resolveErr != nil {
			return workspacecore.Record{}, resolveErr
		}
		environmentDefault = environment.DefaultWorkspace
	}
	return w.runtime.resolveWorkspace(ctx, configuration, selector, environmentDefault)
}
