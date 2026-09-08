package project_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	"github.com/ahillspace/tadx/internal/tableau"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

type pathTestSession struct{}

func (pathTestSession) Authorize(*http.Request) {}
func (pathTestSession) SiteLUID() string        { return "site-1" }
func (pathTestSession) UserLUID() string        { return "user-1" }
func (pathTestSession) String() string          { return "[redacted session]" }

func TestContentResolutionWithLiteralSlashProjectNames(t *testing.T) {
	for _, target := range []struct{ luid, path string }{{"safe", "Shared"}, {"literal", "Ops/Reports"}, {"nested", "Ops/Reports"}} {
		t.Run(target.luid, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				if r.URL.Path == "/api/3.29/sites/site-1/projects" {
					_, _ = fmt.Fprint(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="4"/><projects><project id="safe" name="Shared"/><project id="literal" name="Ops/Reports"/><project id="root" name="Ops"/><project id="nested" name="Reports" parentProjectId="root"/></projects></tsResponse>`)
					return
				}
				for _, kind := range []string{"workbook", "datasource", "flow"} {
					base := "/api/3.29/sites/site-1/" + kind + "s"
					item := fmt.Sprintf(`<%s id="content-1" name="Revenue" contentUrl="Revenue"><project id="%s" name="%s"/></%s>`, kind, target.luid, target.path, kind)
					if r.URL.Path == base+"/content-1" {
						_, _ = fmt.Fprint(w, "<tsResponse>"+item+"</tsResponse>")
						return
					}
					if r.URL.Path == base {
						_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><%ss>%s</%ss></tsResponse>`, kind, item, kind)
						return
					}
				}
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()
			transport := tableau.NewTransport(server.Client(), "3.29", nil)
			projects := resourceproject.NewAdapter(tableauproject.NewClient(transport, pathTestSession{}, server.URL))
			workbooks := resourceworkbook.NewAdapterWithProjectResolver(tableauworkbook.NewClient(transport, pathTestSession{}, server.URL), projects)
			datasources := resourcedatasource.NewAdapterWithProjectResolver(tableaudatasource.NewClient(transport, pathTestSession{}, server.URL), projects)
			flows := resourceflow.NewAdapter(tableauflow.NewClient(transport, pathTestSession{}, server.URL), projects)
			resolvers := map[string]func(identity.Selector) (string, string, error){
				"workbook": func(s identity.Selector) (string, string, error) {
					item, err := workbooks.ResolveWorkbook(context.Background(), s)
					return item.LUID, item.ProjectPath, err
				},
				"datasource": func(s identity.Selector) (string, string, error) {
					item, err := datasources.ResolveDatasource(context.Background(), s)
					return item.LUID, item.ProjectPath, err
				},
				"flow": func(s identity.Selector) (string, string, error) {
					item, err := flows.ResolveFlow(context.Background(), s)
					return item.LUID, item.ProjectPath, err
				},
			}
			for kind, resolve := range resolvers {
				t.Run(kind, func(t *testing.T) {
					id, path, err := resolve(identity.Selector{LUID: "content-1"})
					if err != nil || id != "content-1" || path != target.path {
						t.Errorf("LUID resolution: id=%q path=%q err=%v", id, path, err)
					}
					id, path, err = resolve(identity.Selector{Name: "Revenue", ProjectPath: target.path})
					if target.luid == "safe" {
						if err != nil || id != "content-1" || path != target.path {
							t.Errorf("unique path resolution: id=%q path=%q err=%v", id, path, err)
						}
						return
					}
					var resolution *identity.ResolutionError
					if !errors.As(err, &resolution) || resolution.Kind != identity.ResolutionAmbiguous || !strings.Contains(err.Error(), "literal, nested") {
						t.Errorf("colliding project paths must fail even with only one content match: %v", err)
					}
				})
			}
		})
	}
}
