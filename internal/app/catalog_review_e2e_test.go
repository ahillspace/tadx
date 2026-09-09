package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	flowlist "github.com/ahillspace/tadx/actions/flow/list"
	projectcreate "github.com/ahillspace/tadx/actions/project/create"
	projectmove "github.com/ahillspace/tadx/actions/project/move"
	projectupdate "github.com/ahillspace/tadx/actions/project/update"
	searchaction "github.com/ahillspace/tadx/actions/search"
)

func TestFilteredContentListsRetainCatalogProjectPaths(t *testing.T) {
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
				_, err = commands.ListDatasources(context.Background(), datasourcelist.Input{Environment: "production", Name: "Sales", All: true})
			} else {
				_, err = commands.ListFlows(context.Background(), flowlist.Input{Environment: "production", Name: "Sales", All: true})
			}
			if err != nil {
				t.Fatal(err)
			}
			store := targetCatalogFixture(t, runtime.configPath, runtime.now)
			out, err := searchaction.New(catalogGlobalSearchSource{store: store}).Execute(context.Background(), searchaction.Input{Type: kind, Environment: "production", Site: "team-site", SiteResolved: true, Catalog: true, ProjectPath: "Department/Ops"})
			if err != nil || len(out.Items) != 1 || out.Items[0].ProjectPath != "Department/Ops" {
				t.Fatalf("cached filtered search = %#v, %v", out, err)
			}
		})
	}
}

func TestNativeSearchReportsTotalAndResultWindowWarning(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case r.URL.Path == "/api/-/search":
			_, _ = io.WriteString(w, `{"items":[{"uri":"/workbooks/wb-1","content":{"luid":"wb-1","contentType":"WORKBOOK","name":"Sales"}}],"limit":1,"pageIndex":0,"startIndex":0,"total":2001,"next":"/api/-/search?page=1"}`)
		default:
			http.Error(w, "unexpected request", 404)
		}
	}))
	defer server.Close()
	out, err := newSearchCommands(inventoryListRuntime(t, server)).Execute(context.Background(), searchaction.Input{Environment: "production", Type: "workbook", Terms: "Sales", Limit: 1})
	if err != nil || out.Page.Total != 2001 || out.Page.NextCursor == "" || !strings.Contains(strings.Join(out.Warnings, " "), "2,000") {
		t.Fatalf("search result = %#v, %v", out, err)
	}
}

func TestProjectMutationsSucceedWhenPostMutationHierarchyIsUnavailable(t *testing.T) {
	for _, operation := range []string{"create", "update", "move", "update-unconfirmed-parent"} {
		t.Run(operation, func(t *testing.T) {
			var mutated atomic.Bool
			var postReads atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/auth/signin"):
					_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects"):
					if mutated.Load() {
						postReads.Add(1)
						http.Error(w, "hierarchy unavailable", 403)
						return
					}
					_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="3"/><projects><project id="parent" name="Department"/><project id="destination" name="Destination"/><project id="project-1" name="Old" parentProjectId="parent"/></projects></tsResponse>`)
				case r.Method == http.MethodPost || r.Method == http.MethodPut:
					mutated.Store(true)
					name, parent := "New", "parent"
					if operation == "update-unconfirmed-parent" {
						parent = "unconfirmed-parent"
					}
					if operation == "move" {
						name, parent = "Old", "destination"
					}
					w.Header().Set("X-Tableau-Request-Id", "mutation-evidence")
					if operation == "create" {
						w.WriteHeader(http.StatusCreated)
					}
					_, _ = fmt.Fprintf(w, `<tsResponse><project id="project-1" name="%s" parentProjectId="%s"/></tsResponse>`, name, parent)
				default:
					http.Error(w, "unexpected request", 404)
				}
			}))
			defer server.Close()
			commands := newRemoteContentCommands(inventoryListRuntime(t, server))
			var path, requestID string
			var help []string
			var err error
			switch operation {
			case "create":
				in := projectcreate.Input{Environment: "production", Name: "New"}
				in.SetParentSelector("parent", "")
				out, callErr := commands.CreateProject(context.Background(), in, false)
				err = callErr
				if out.Result != nil {
					path, requestID = out.Result.Project.Path, out.Result.TableauRequestID
				}
			case "update", "update-unconfirmed-parent":
				name := "New"
				in := projectupdate.Input{Environment: "production", Name: &name}
				in.SetSelector("project-1", "")
				out, callErr := commands.UpdateProject(context.Background(), in, false)
				err = callErr
				help = out.Help
				if out.Result != nil {
					path, requestID = out.Result.Project.Path, out.Result.TableauRequestID
				}
			case "move":
				in := projectmove.Input{Environment: "production"}
				in.SetProjectSelector("project-1", "")
				in.SetParentSelector("destination", "")
				out, callErr := commands.MoveProject(context.Background(), in, false)
				err = callErr
				if out.Result != nil {
					path, requestID = out.Result.Project.Path, out.Result.TableauRequestID
				}
			}
			want := "Department/New"
			if operation == "update-unconfirmed-parent" {
				if err != nil || path != "" || requestID != "mutation-evidence" || !containsString(help, projectMutationPathWarning) || postReads.Load() != 1 {
					t.Fatalf("unconfirmed mutation: path=%q request=%q help=%v reads=%d err=%v", path, requestID, help, postReads.Load(), err)
				}
				return
			}
			if operation == "move" {
				want = "Destination/Old"
			}
			if err != nil || path != want || requestID != "mutation-evidence" || !mutated.Load() || postReads.Load() != 0 {
				t.Fatalf("mutation result: path=%q request=%q mutated=%t postReads=%d err=%v", path, requestID, mutated.Load(), postReads.Load(), err)
			}
		})
	}
}
