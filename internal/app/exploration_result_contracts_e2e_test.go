package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func resultContractJSON(t *testing.T, args []string, options app.Options) (int, map[string]any) {
	t.Helper()
	var output bytes.Buffer
	code := app.Run(t.Context(), append(args, "--json"), &output, options)
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON (exit %d): %s: %v", code, output.String(), err)
	}
	return code, payload
}

func contractObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %#v", value)
	}
	return object
}

func TestExplorationF02GroupUpdatePreviewShowsValuesThroughCLI(t *testing.T) {
	reads := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/3.29/sites/site-1/groups" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", 500)
			return
		}
		reads++
		_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><groups><group id="group-1" name="Before" minimumSiteRole="Viewer" externalUserEnabled="false"/></groups></tsResponse>`, r.URL.Query().Get("pageSize"))
	}))
	defer server.Close()
	code, payload := resultContractJSON(t, []string{"admin", "group", "update", "--environment", "test", "--id", "group-1", "--new-name", "After", "--minimum-site-role", "Explorer", "--external-user-enabled", "--preview"}, diagnosticOptions(t, server))
	if code != 0 || reads != 1 {
		t.Fatalf("exit=%d reads=%d payload=%#v", code, reads, payload)
	}
	if payload["plan"] == nil {
		t.Fatalf("preview plan missing: %#v", payload)
	}
	plan := contractObject(t, payload["plan"])
	requested := contractObject(t, plan["requested"])
	if requested["name"] != "After" || requested["minimum_site_role"] != "Explorer" || requested["external_user_enabled"] != true {
		t.Fatalf("requested values missing: %#v", plan)
	}
	changes, ok := plan["changes"].([]any)
	if !ok || len(changes) != 3 {
		t.Fatalf("confirmed previous values missing: %#v", plan)
	}
	confirmed := map[string][2]string{}
	for _, value := range changes {
		change := contractObject(t, value)
		confirmed[change["field"].(string)] = [2]string{change["before"].(string), change["after"].(string)}
	}
	if confirmed["name"] != [2]string{"Before", "After"} || confirmed["minimum_site_role"] != [2]string{"Viewer", "Explorer"} || confirmed["external_user_enabled"] != [2]string{"false", "true"} {
		t.Fatalf("incorrect confirmed before/after values: %#v", confirmed)
	}
}

func TestExplorationF03CatalogInspectFailureStatusThroughCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if catalogMetadataSignIn(w, r) {
			return
		}
		if r.URL.Path != "/api/metadata/graphql" {
			t.Errorf("unexpected %s", r.URL)
			http.Error(w, "unexpected", 500)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"items":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}},"errors":[{"message":"coverage denied"}]}`)
	}))
	defer server.Close()
	options := catalogMetadataOptions(t, server, false)
	for _, kind := range []string{"database", "table", "column"} {
		code, payload := resultContractJSON(t, []string{"catalog", kind, "inspect", "--environment", "production", "--metadata-id", "metadata-db"}, options)
		if code == 0 {
			t.Fatalf("%s expected failure: %#v", kind, payload)
		}
		output := contractObject(t, payload["output"])
		if output["status"] != "failed" {
			t.Fatalf("%s contradictory inspect status: %#v", kind, payload)
		}
	}
}

func TestExplorationF04ExactTrailingWorkbookNameThroughCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/3.29/sites/site-1/projects":
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Ops"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"))
		case "/api/3.29/sites/site-1/workbooks":
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><workbooks><workbook id="workbook-1" name="Finance " contentUrl="Finance"><project id="project-1" name="Ops"/><owner id="user-1"/></workbook></workbooks></tsResponse>`, r.URL.Query().Get("pageSize"))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", 500)
		}
	}))
	defer server.Close()
	options := diagnosticOptions(t, server)
	code, payload := resultContractJSON(t, []string{"content", "workbook", "list", "--environment", "test", "--name", "Finance ", "--full"}, options)
	if code != 0 {
		t.Fatalf("list exit=%d payload=%#v", code, payload)
	}
	code, payload = resultContractJSON(t, []string{"content", "workbook", "inspect", "--environment", "test", "--name", "Finance ", "--project-id", "project-1"}, options)
	if code != 0 {
		t.Fatalf("exact inspect exit=%d payload=%#v", code, payload)
	}
	if !strings.Contains(fmt.Sprint(payload), "Finance ") {
		t.Fatalf("name lost trailing space: %#v", payload)
	}
}

func TestExplorationF04CachedWorkbookNamePreservesTrailingSpaceThroughCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/3.29/sites/site-1/workbooks":
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><workbooks><workbook id="workbook-1" name="Finance "><project id="project-1" name="Ops"/><owner id="user-1"/></workbook></workbooks></tsResponse>`)
		case "/api/3.29/sites/site-1/projects":
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Ops"/></projects></tsResponse>`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", 500)
		}
	}))
	defer server.Close()
	options := diagnosticOptions(t, server)
	code, payload := resultContractJSON(t, []string{"cache", "refresh", "--environment", "test", "--scope", "workbooks"}, options)
	if code != 0 {
		t.Fatalf("refresh exit=%d payload=%#v", code, payload)
	}
	code, payload = resultContractJSON(t, []string{"content", "workbook", "inspect", "--environment", "test", "--id", "workbook-1", "--cache", "--full"}, options)
	if code != 0 {
		t.Fatalf("cached inspect exit=%d payload=%#v", code, payload)
	}
	workbook := contractObject(t, payload["workbook"])
	if workbook["name"] != "Finance " {
		t.Fatalf("cached workbook name changed: %#v", payload)
	}
}

func TestExplorationF05ProjectOwnerLUIDThroughCLI(t *testing.T) {
	const owner = "1f876ad6-d65f-4b4e-9c67-3fbbe38fdd37"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		if r.URL.Path != "/api/3.29/sites/site-1/projects" {
			t.Errorf("unexpected %s", r.URL)
			http.Error(w, "unexpected", 500)
			return
		}
		if strings.Contains(r.URL.Query().Get("filter"), "ownerName:eq:"+owner) {
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="0"/><projects/></tsResponse>`, r.URL.Query().Get("pageSize"))
			return
		}
		_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="2"/><projects><project id="project-1" name="Ops"><owner id="%s"/></project><project id="project-2" name="Other"><owner id="other-owner"/></project></projects></tsResponse>`, r.URL.Query().Get("pageSize"), owner)
	}))
	defer server.Close()
	code, payload := resultContractJSON(t, []string{"content", "project", "list", "--environment", "test", "--owner", owner, "--full"}, diagnosticOptions(t, server))
	if code != 0 {
		t.Fatalf("exit=%d payload=%#v", code, payload)
	}
	projects, ok := payload["projects"].([]any)
	if !ok || len(projects) != 1 || contractObject(t, projects[0])["luid"] != "project-1" {
		t.Fatalf("owner LUID filter: %#v", payload)
	}
	code, payload = resultContractJSON(t, []string{"content", "project", "list", "--environment", "test", "--owner", owner, "--all"}, diagnosticOptions(t, server))
	if code != 0 {
		t.Fatalf("owner LUID --all exit=%d payload=%#v", code, payload)
	}
	projects, ok = payload["projects"].([]any)
	if !ok || len(projects) != 1 || contractObject(t, projects[0])["luid"] != "project-1" {
		t.Fatalf("owner LUID --all: %#v", payload)
	}
}

func TestExplorationF07KnownEmptyPulseDefinitionsThroughCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		if r.URL.Path != "/api/-/pulse/definitions" {
			t.Errorf("unexpected %s", r.URL)
			http.Error(w, "unexpected", 500)
			return
		}
		_, _ = io.WriteString(w, `{"definitions":[]}`)
	}))
	defer server.Close()
	for _, full := range []bool{false, true} {
		args := []string{"pulse", "definition", "list", "--environment", "test", "--name", "missing"}
		if full {
			args = append(args, "--full")
		}
		code, payload := resultContractJSON(t, args, diagnosticOptions(t, server))
		if code != 0 {
			t.Fatalf("full=%v exit=%d payload=%#v", full, code, payload)
		}
		definitions, ok := payload["definitions"].([]any)
		if !ok || len(definitions) != 0 {
			t.Fatalf("full=%v known empty definitions: %#v", full, payload)
		}
	}
}
