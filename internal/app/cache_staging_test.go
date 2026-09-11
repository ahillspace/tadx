package app

import (
	"context"
	"testing"
	"time"

	cacherefresh "github.com/ahillspace/tadx/actions/cache/refresh"
	corecache "github.com/ahillspace/tadx/internal/cache"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

func TestCacheCollectionDoesNotLockUnrelatedEnvironmentWrites(t *testing.T) {
	ctx := context.Background()
	store := corecache.NewStore(t.TempDir(), time.Now)
	if err := store.UpsertResources(ctx, []corecache.ResourceEntry{{Environment: "other", Site: "site", Kind: "user", LUID: "old", Name: "Old", Coverage: "summary", ObservedAt: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	collecting, release := make(chan struct{}), make(chan struct{})
	runner := cacheRunnerFunc(func(ctx context.Context, _ tableaucache.RunRequest, writer tableaucache.BatchWriter) (tableaucache.Result, error) {
		if err := writer.WriteBatch(ctx, tableaucache.Batch{Scope: tableaucache.ScopeProjects, Columns: mustCacheColumns(t, tableaucache.ScopeProjects), Rows: [][]any{{"project", "Project", "", "", "", `{}`}}}); err != nil {
			return tableaucache.Result{}, err
		}
		close(collecting)
		<-release
		return tableaucache.Result{RequestedScopes: []tableaucache.Scope{tableaucache.ScopeProjects}, Counts: map[tableaucache.Scope]int64{tableaucache.ScopeProjects: 1}}, nil
	})
	hydrator := cacheHydrator{store: store, now: time.Now, executorFor: func(context.Context, string, string) (tableaucache.Executor, error) { return nil, nil }, newRunner: func(tableaucache.Executor) (cacheRunner, error) { return runner, nil }}
	hydrated := make(chan error, 1)
	go func() {
		_, err := hydrator.Hydrate(ctx, cacherefresh.HydrationRequest{Environment: "refresh", Site: "site", RequestedScopes: []string{"projects"}})
		hydrated <- err
	}()
	select {
	case <-collecting:
	case err := <-hydrated:
		t.Fatalf("collector did not start: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("collector did not start")
	}
	written := make(chan error, 1)
	go func() {
		written <- store.UpsertResources(ctx, []corecache.ResourceEntry{{Environment: "other", Site: "site", Kind: "user", LUID: "new", Name: "New", Coverage: "summary", ObservedAt: time.Now()}})
	}()
	blocked := false
	select {
	case err := <-written:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(time.Second):
		blocked = true
	}
	close(release)
	if err := <-hydrated; err != nil {
		t.Fatal(err)
	}
	if blocked {
		if err := <-written; err != nil {
			t.Error(err)
		}
		t.Fatal("network collection held the active cache write lock")
	}
}
