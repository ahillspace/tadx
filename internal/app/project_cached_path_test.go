package app

import (
	"context"
	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/identity"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProjectInspectCacheRetainsCanonicalPathAfterLiveList(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/signin") {
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		} else {
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="root" name="Department"/><project id="child" name="Ops" parentProjectId="root" description="Keep this detail"/></projects></tsResponse>`)
		}
	}))
	defer server.Close()
	commands := newRemoteContentCommands(inventoryListRuntime(t, server))
	if _, err := commands.dependencies().ProjectLister.ListProjects(context.Background(), projectops.ListInput{Environment: "production", All: true}); err != nil {
		t.Fatal(err)
	}
	got, err := commands.dependencies().ProjectInspector.InspectProject(context.Background(), projectops.InspectInput{Environment: "production", Cache: true, Selector: identity.Selector{LUID: "child"}})
	if err != nil || got.Project.Path != "Department/Ops" || got.Project.Description != "Keep this detail" {
		t.Fatalf("cache inspect = %#v %v", got, err)
	}
}

func TestProjectInspectCacheRetainsCanonicalPathAfterFullRefresh(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	runtime := inventoryListRuntime(t, server)
	store := targetCacheFixture(t, runtime.configPath, runtime.now)
	writer, err := store.BeginGeneration(ctx, cache.GenerationMetadata{Environment: "production", Site: "team-site", GeneratedAt: runtime.now(), Source: "tableau-rest", RequestedScopes: []string{"projects"}})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if err := writer.WriteBatch(ctx, cache.Batch{Scope: "projects", Columns: []string{"id", "name", "parent_project_id", "description", "owner_id", "list_payload"}, Rows: [][]any{
		{"root", "Department", "", "", "", `{}`},
		{"child", "Ops", "root", "Keep this detail", "owner", `{"luid":"child","name":"Ops","parent_luid":"root","description":"Keep this detail"}`},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.CompleteScopes(ctx, []string{"projects"}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := newRemoteContentCommands(runtime).dependencies().ProjectInspector.InspectProject(ctx, projectops.InspectInput{Environment: "production", Cache: true, Selector: identity.Selector{LUID: "child"}})
	if err != nil || got.Project.Path != "Department/Ops" || got.Project.Description != "Keep this detail" || got.Project.OwnerLUID != "owner" {
		t.Fatalf("cache inspect = %#v %v", got, err)
	}
}
