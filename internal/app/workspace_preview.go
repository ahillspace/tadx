package app

import (
	"context"
	workspacesetdefault "github.com/ahillspace/tadx/actions/workspace/setdefault"
	workspaceunregister "github.com/ahillspace/tadx/actions/workspace/unregister"
)

func (a workspaceDefaultStore) PreviewSetDefault(ctx context.Context, name string) (workspacesetdefault.Workspace, error) {
	item, err := a.runtime.manager().PreviewSetDefault(ctx, name)
	return workspacesetdefault.Workspace{Name: item.Name, ID: item.ID, Root: item.Root}, err
}

func (a workspaceRegistryStore) PreviewUnregister(ctx context.Context, name string) (workspaceunregister.Workspace, error) {
	item, err := a.runtime.manager().PreviewUnregister(ctx, name)
	return workspaceunregister.Workspace{Name: item.Name, ID: item.ID, Root: item.Root}, err
}
