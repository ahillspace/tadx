package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	searchaction "github.com/ahillspace/tadx/actions/search"
)

func executeSearchAction(ctx context.Context, source searchaction.Source, input searchaction.Input) (searchaction.Output, error) {
	types, err := searchaction.ValidateInput(input)
	if err != nil {
		return searchaction.Output{}, err
	}
	return searchaction.Execute(ctx, source, input, types)
}

func TestSearchCommandsUseNativeEndpointForContentTerms(t *testing.T) {
	searchCalls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/-/search":
			searchCalls++
			if request.URL.Query().Get("terms") != "Sales" || request.URL.Query().Get("filter") != "type:eq:workbook" {
				t.Errorf("native query=%q", request.URL.RawQuery)
			}
			_, _ = io.WriteString(writer, `{"items":[{"uri":"/workbooks/wb-1","content":{"type":"workbook","luid":"wb-1","title":"Sales Dashboard"}}],"limit":20,"pageIndex":0,"startIndex":0,"total":1}`)
		default:
			t.Errorf("unexpected legacy request %s %s", request.Method, request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf("version: 1\ndefault_environment: production\nenvironments:\n  production:\n    url: %s\n    site_content_url: \"\"\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PROD_PAT_NAME\n      pat_secret_env: PROD_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	code := Run(context.Background(), []string{"search", "Sales", "--type", "workbook", "--environment", "production"}, &stdout, Options{ConfigPath: configPath, HTTPClient: server.Client()})
	if code != 0 || searchCalls != 1 || !strings.Contains(stdout.String(), "wb-1,workbook,Sales Dashboard") || !strings.Contains(stdout.String(), "source: live") {
		t.Fatalf("code=%d calls=%d output=%s", code, searchCalls, stdout.String())
	}
}

func TestBlankTypedSearchUsesBoundedLivePagesWithoutCache(t *testing.T) {
	reads := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Sales"/></projects></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/workbooks"):
			reads++
			number := r.URL.Query().Get("pageNumber")
			if r.URL.Query().Get("pageSize") != "1" {
				t.Errorf("unbounded request %s", r.URL.RawQuery)
			}
			id, name := "workbook-a", "Alpha"
			if number == "2" {
				id, name = "workbook-b", "Beta"
			}
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="1" totalAvailable="2"/><workbooks><workbook id="%s" name="%s"><project id="project-1"/></workbook></workbooks></tsResponse>`, number, id, name)
		default:
			http.Error(w, "unexpected request", 404)
		}
	}))
	defer server.Close()
	runtime := inventoryListRuntime(t, server)
	commands := newSearchCommands(runtime)
	input := searchaction.Input{Environment: "production", Type: "workbook", Limit: 1}
	first, err := commands.Execute(context.Background(), input)
	if err != nil || reads != 1 || len(first.Items) != 1 || first.Items[0].LUID != "workbook-a" || !first.Page.MoreAvailable {
		t.Fatalf("first=%+v reads=%d err=%v", first, reads, err)
	}
	input.Cursor = first.Page.NextCursor
	second, err := commands.Execute(context.Background(), input)
	if err != nil || reads != 2 || len(second.Items) != 1 || second.Items[0].LUID != "workbook-b" || second.Page.MoreAvailable {
		t.Fatalf("second=%+v reads=%d err=%v", second, reads, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(runtime.configPath), targetCacheFixture(t, runtime.configPath, runtime.now).RelativePath())); !os.IsNotExist(err) {
		t.Fatalf("limited search created cache: %v", err)
	}
}
