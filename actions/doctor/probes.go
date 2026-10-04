package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/workspace"
)

// CacheTarget contains the selected store and canonical cache scope.
type CacheTarget struct {
	Store     *cache.Store
	Selection cache.Selection
}

// CacheTargetResolver binds the selected environment at probe time.
type CacheTargetResolver interface {
	ResolveCacheTarget(Scope) (CacheTarget, error)
}

// CacheProbe owns local cache health interpretation.
type CacheProbe struct {
	configPath func() string
	resolver   CacheTargetResolver
}

// NewCacheProbe constructs a cache probe with invocation-time configuration.
func NewCacheProbe(configPath func() string, resolver CacheTargetResolver) *CacheProbe {
	return &CacheProbe{configPath: configPath, resolver: resolver}
}

func (p *CacheProbe) CheckCache(ctx context.Context, scope Scope) (CacheState, error) {
	target, err := p.resolver.ResolveCacheTarget(scope)
	if err != nil {
		return CacheState{}, err
	}
	database := filepath.Join(filepath.Dir(p.configPath()), filepath.FromSlash(target.Store.RelativePath()))
	if _, err := os.Stat(database); errors.Is(err, os.ErrNotExist) {
		return CacheState{}, nil
	} else if err != nil {
		return CacheState{}, err
	}
	status, err := target.Store.Status(ctx, target.Selection)
	if err != nil {
		return CacheState{Present: true}, err
	}
	return CacheState{Present: true, Complete: status.Complete, Stale: status.Stale}, nil
}

// WorkspaceTargetResolver binds the selected workspace at probe time.
type WorkspaceTargetResolver interface {
	ResolveWorkspaceTarget(context.Context, Scope) (workspace.Record, error)
}

// WorkspaceProbe owns bounded artifact-health interpretation.
type WorkspaceProbe struct{ resolver WorkspaceTargetResolver }

// NewWorkspaceProbe constructs a workspace probe.
func NewWorkspaceProbe(resolver WorkspaceTargetResolver) *WorkspaceProbe {
	return &WorkspaceProbe{resolver: resolver}
}

func (p *WorkspaceProbe) CheckWorkspace(ctx context.Context, scope Scope) (WorkspaceState, error) {
	selected, err := p.resolver.ResolveWorkspaceTarget(ctx, scope)
	if err != nil {
		return WorkspaceState{}, err
	}
	state := WorkspaceState{Available: selected.Available, ManifestValid: selected.ManifestValid}
	if !selected.Available || !selected.ManifestValid {
		return state, nil
	}
	inventory, err := artifact.Inventory(ctx, selected.Root, artifact.InventoryOptions{Limit: 1000})
	if err != nil {
		return state, err
	}
	state.DirtyArtifacts = inventory.Dirty + inventory.Missing + inventory.Invalid
	return state, nil
}
