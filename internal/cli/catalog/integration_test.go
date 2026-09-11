package catalog

import (
	"context"
	databaseupdate "github.com/ahillspace/tadx/actions/catalog/database/update"
	catalogresource "github.com/ahillspace/tadx/internal/resources/catalog"
	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/tableau/metadataassets"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type session struct{}

func (session) Authorize(r *http.Request) { r.Header.Set("X-Tableau-Auth", "fixture-token") }
func (session) SiteLUID() string          { return "site" }
func (session) UserLUID() string          { return "user" }
func (session) String() string            { return "fixture session" }

type updateService struct {
	action *databaseupdate.Action
	last   databaseupdate.Output
}

func (s *updateService) UpdateCatalogDatabase(ctx context.Context, in databaseupdate.Input, preview bool) (databaseupdate.Output, error) {
	in.Environment = "fixture"
	in.Site = "site"
	in.TargetResolved = true
	out, e := s.action.Execute(ctx, in, preview)
	s.last = out
	return out, e
}
func TestCLIUpdatePreviewAndPartialMutationHTTP(t *testing.T) {
	reads, writes := 0, 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tableau-Auth") != "fixture-token" {
			t.Error("session not reused")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site/databases/db":
			reads++
			io.WriteString(w, `<tsResponse><database id="db" name="Warehouse" description="Before"><contact id="keep-contact"/><tags><tag label="keep-tag"/></tags></database></tsResponse>`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site/databases/db":
			writes++
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `description="After"`) || strings.Contains(string(body), "contact") || strings.Contains(string(body), "tags") {
				t.Errorf("unrelated properties changed: %s", body)
			}
			io.WriteString(w, `<tsResponse><database id="db" name="Warehouse" description="After"/></tsResponse>`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site/databases/db/tags":
			writes++
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `<tsResponse><error code="403004"><summary>Forbidden</summary><detail>Tag update denied</detail></error></tsResponse>`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	adapter := catalogresource.New(metadataassets.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL))
	service := &updateService{action: databaseupdate.New(adapter, adapter)}
	renderer := &recorder{}
	command := New(Dependencies{DatabaseUpdater: service, Renderer: renderer})
	command.SetArgs([]string{"database", "update", "--id", "db", "--description", "After", "--add-tag", "new", "--preview"})
	if e := command.Execute(); e != nil {
		t.Fatal(e)
	}
	if writes != 0 || reads != 1 {
		t.Fatalf("preview made %d reads %d writes", reads, writes)
	}
	command = New(Dependencies{DatabaseUpdater: service, Renderer: renderer})
	command.SetArgs([]string{"database", "update", "--id", "db", "--description", "After", "--add-tag", "new"})
	if e := command.Execute(); e == nil {
		t.Fatal("expected tag failure")
	}
	if service.last.Result == nil || service.last.Result.Identity.LUID != "db" || len(service.last.Result.Completed) != 1 || service.last.Result.Completed[0] != "description" {
		t.Fatalf("lost confirmed partial result: %+v", service.last)
	}
	if writes != 2 || reads != 3 {
		t.Fatalf("unexpected retries: %d reads %d writes", reads, writes)
	}
}
