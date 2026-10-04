package app

import (
	"context"
	"fmt"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	searchaction "github.com/ahillspace/tadx/actions/search"
	resourcesearch "github.com/ahillspace/tadx/internal/resources/search"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFilteredContentListsRetainCacheProjectPaths(t *testing.T) {
	for _, kind := range []string{"datasource", "flow"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/auth/signin"):
					_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case strings.HasSuffix(r.URL.Path, "/projects"):
					_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="root" name="Department"/><project id="child" name="Ops" parentProjectId="root"/></projects></tsResponse>`)
				case strings.HasSuffix(r.URL.Path, "/"+kind+"s"):
					_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><%ss><%s id="item-1" name="Sales" type="sqlserver" fileType="tfl"><project id="child" name="Ops"/><owner id="user-1"/></%s></%ss></tsResponse>`, kind, kind, kind, kind)
				default:
					http.Error(w, "unexpected request", 404)
				}
			}))
			defer server.Close()
			runtime := inventoryListRuntime(t, server)
			commands := newRemoteContentCommands(runtime)
			var err error
			if kind == "datasource" {
				_, err = commands.dependencies().DatasourceLister.ListDatasources(context.Background(), datasourceops.ListInput{Environment: "production", Name: "Sales", All: true})
			} else {
				_, err = commands.dependencies().FlowLister.ListFlows(context.Background(), flowops.ListInput{Environment: "production", Name: "Sales", All: true})
			}
			if err != nil {
				t.Fatal(err)
			}
			store := targetCacheFixture(t, runtime.configPath, runtime.now)
			out, err := executeSearchAction(context.Background(), resourcesearch.CacheSource{Store: store}, searchaction.Input{Type: kind, Environment: "production", Site: "team-site", SiteResolved: true, Cache: true, ProjectPath: "Department/Ops"})
			if err != nil || len(out.Items) != 1 || out.Items[0].ProjectPath != "Department/Ops" {
				t.Fatalf("cached filtered search = %#v, %v", out, err)
			}
		})
	}
}
