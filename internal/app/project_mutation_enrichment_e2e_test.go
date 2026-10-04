package app

import (
	"context"
	"fmt"
	projectops "github.com/ahillspace/tadx/actions/project"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

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
				in := projectops.CreateInput{Environment: "production", Name: "New"}
				in.SetParentSelector("parent", "")
				out, callErr := commands.dependencies().ProjectCreator.CreateProject(context.Background(), in, false)
				err = callErr
				if out.Result != nil {
					path, requestID = out.Result.Project.Path, out.Result.TableauRequestID
				}
			case "update", "update-unconfirmed-parent":
				name := "New"
				in := projectops.UpdateInput{Environment: "production", Name: &name}
				in.SetSelector("project-1", "")
				out, callErr := commands.dependencies().ProjectUpdater.UpdateProject(context.Background(), in, false)
				err = callErr
				help = out.Help
				if out.Result != nil {
					path, requestID = out.Result.Project.Path, out.Result.TableauRequestID
				}
			case "move":
				in := projectops.MoveInput{Environment: "production"}
				in.SetProjectSelector("project-1", "")
				in.SetParentSelector("destination", "")
				out, callErr := commands.dependencies().ProjectMover.MoveProject(context.Background(), in, false)
				err = callErr
				if out.Result != nil {
					path, requestID = out.Result.Project.Path, out.Result.TableauRequestID
				}
			}
			want := "Department/New"
			if operation == "update-unconfirmed-parent" {
				if err != nil || path != "" || requestID != "mutation-evidence" || !containsString(help, projectops.MutationPathWarning) || postReads.Load() != 1 {
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
