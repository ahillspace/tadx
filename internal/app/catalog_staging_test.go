package app

import (
	"context"
	"testing"
	"time"

	catalogrefresh "github.com/ahillspace/tadx/actions/catalog/refresh"
	corecatalog "github.com/ahillspace/tadx/internal/catalog"
	tableaucatalog "github.com/ahillspace/tadx/internal/tableau/catalog"
)

func TestCatalogCollectionDoesNotLockUnrelatedEnvironmentWrites(t *testing.T) {
	ctx := context.Background()
	store := corecatalog.NewStore(t.TempDir(), time.Now)
	if err := store.UpsertResources(ctx, []corecatalog.ResourceEntry{{Environment: "other", Site: "site", Kind: "user", LUID: "old", Name: "Old", Coverage: "summary", ObservedAt: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	collecting, release := make(chan struct{}), make(chan struct{})
	runner := catalogRunnerFunc(func(ctx context.Context, _ tableaucatalog.RunRequest, writer tableaucatalog.BatchWriter) (tableaucatalog.Result, error) {
		if err := writer.WriteBatch(ctx, tableaucatalog.Batch{Scope: tableaucatalog.ScopeProjects, Columns: mustCatalogColumns(t, tableaucatalog.ScopeProjects), Rows: [][]any{{"project", "Project", "", "", "", `{}`}}}); err != nil {
			return tableaucatalog.Result{}, err
		}
		close(collecting)
		<-release
		return tableaucatalog.Result{RequestedScopes: []tableaucatalog.Scope{tableaucatalog.ScopeProjects}, Counts: map[tableaucatalog.Scope]int64{tableaucatalog.ScopeProjects: 1}}, nil
	})
	hydrator := catalogHydrator{store: store, now: time.Now, executorFor: func(context.Context, string, string) (tableaucatalog.Executor, error) { return nil, nil }, newRunner: func(tableaucatalog.Executor) (catalogRunner, error) { return runner, nil }}
	hydrated := make(chan error, 1)
	go func() {
		_, err := hydrator.Hydrate(ctx, catalogrefresh.HydrationRequest{Environment: "refresh", Site: "site", RequestedScopes: []string{"projects"}})
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
		written <- store.UpsertResources(ctx, []corecatalog.ResourceEntry{{Environment: "other", Site: "site", Kind: "user", LUID: "new", Name: "New", Coverage: "summary", ObservedAt: time.Now()}})
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
		t.Fatal("network collection held the active catalog write lock")
	}
}
