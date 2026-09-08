package project_test

import (
	"context"
	"testing"

	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/resources/project"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
)

type discoveryProjectsClient struct {
	calls int
	root  string
}

func (c *discoveryProjectsClient) List(_ context.Context, in tableauproject.ListRequest) (tableauproject.Page, error) {
	c.calls++
	return tableauproject.Page{Number: in.PageNumber, Size: in.PageSize, Total: 3, Items: []tableauproject.Project{{LUID: "root", Name: c.root}, {LUID: "child", Name: "Orders", ParentLUID: "root"}, {LUID: "unrelated", Name: "Unrelated", ParentLUID: "missing"}}}, nil
}

func TestDiscoveryHierarchyIsLazyImmutableAndNeverChangesLiveResolution(t *testing.T) {
	ctx := context.Background()
	client := &discoveryProjectsClient{root: "Original"}
	adapter := project.NewAdapter(client)
	discovery := project.NewDiscoveryPaths(adapter)
	if _, err := discovery.ResolveProjectPaths(ctx, nil); err != nil || client.calls != 0 {
		t.Fatalf("empty selection fetched hierarchy: calls=%d err=%v", client.calls, err)
	}
	paths, err := discovery.ResolveProjectPaths(ctx, []string{"child"})
	if err != nil || paths["child"] != "Original/Orders" || client.calls != 1 {
		t.Fatalf("initial paths=%v calls=%d err=%v", paths, client.calls, err)
	}
	paths["child"] = "caller mutation"
	client.root = "Fresh"
	paths, err = discovery.ResolveProjectPaths(ctx, []string{"child", "root"})
	if err != nil || paths["child"] != "Original/Orders" || paths["root"] != "Original" || client.calls != 1 {
		t.Fatalf("discovery was mutable or fetched again: paths=%v calls=%d err=%v", paths, client.calls, err)
	}
	live, err := adapter.ResolveProject(ctx, identity.Selector{LUID: "child"})
	if err != nil || live.Path != "Fresh/Orders" || client.calls != 2 {
		t.Fatalf("live identity resolution reused discovery state: path=%s calls=%d err=%v", live.Path, client.calls, err)
	}
	fresh := project.NewDiscoveryPaths(adapter)
	paths, err = fresh.ResolveProjectPaths(ctx, []string{"child"})
	if err != nil || paths["child"] != "Fresh/Orders" || client.calls != 3 {
		t.Fatalf("new discovery reused prior invocation: paths=%v calls=%d err=%v", paths, client.calls, err)
	}
	if _, err = discovery.ResolveProjectPaths(ctx, []string{"unrelated"}); err == nil {
		t.Fatal("requested invalid hierarchy was accepted")
	}
}
