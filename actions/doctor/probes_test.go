package doctor

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/workspace"
)

type cacheTargetFixture struct {
	target CacheTarget
	err    error
	calls  int
}

func (f *cacheTargetFixture) ResolveCacheTarget(Scope) (CacheTarget, error) {
	f.calls++
	return f.target, f.err
}

func TestCacheProbeKeepsMissingStoreOptionalAndResolverErrors(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	store := cache.NewTargetStore(filepath.Dir(configPath), "https://example.test", "site", time.Now)
	resolver := &cacheTargetFixture{target: CacheTarget{
		Store: store, Selection: cache.Selection{Environment: "dev", Site: "site", SiteSelected: true},
	}}
	probe := NewCacheProbe(func() string { return configPath }, resolver)
	state, err := probe.CheckCache(t.Context(), Scope{Environment: "dev"})
	if err != nil || state.Present || resolver.calls != 1 {
		t.Fatalf("missing cache state=%+v err=%v calls=%d", state, err, resolver.calls)
	}
	want := errors.New("selected environment unavailable")
	resolver.err = want
	state, err = probe.CheckCache(t.Context(), Scope{Environment: "missing"})
	if !errors.Is(err, want) || state.Present || resolver.calls != 2 {
		t.Fatalf("resolver state=%+v err=%v calls=%d", state, err, resolver.calls)
	}
}

type workspaceTargetFixture struct {
	record workspace.Record
	err    error
}

func (f workspaceTargetFixture) ResolveWorkspaceTarget(context.Context, Scope) (workspace.Record, error) {
	return f.record, f.err
}

func TestWorkspaceProbeDoesNotInventoryUnavailableWorkspace(t *testing.T) {
	probe := NewWorkspaceProbe(workspaceTargetFixture{record: workspace.Record{
		Root: filepath.Join(t.TempDir(), "missing"), Available: false, ManifestValid: true,
	}})
	state, err := probe.CheckWorkspace(t.Context(), Scope{Workspace: "missing"})
	if err != nil || state.Available || !state.ManifestValid || state.DirtyArtifacts != 0 {
		t.Fatalf("state=%+v err=%v", state, err)
	}
}
