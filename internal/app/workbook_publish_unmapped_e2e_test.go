package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/app"
)

// datasourceReference is one published datasource a test workbook uses.
type datasourceReference struct{ caption, contentURL string }

func workbookWithDatasources(references ...datasourceReference) string {
	var b strings.Builder
	b.WriteString("<?xml version='1.0' encoding='utf-8' ?>\n<workbook version='18.1'>\n  <datasources>\n")
	for index, reference := range references {
		caption := strings.ReplaceAll(reference.caption, "&", "&amp;")
		fmt.Fprintf(&b, "    <datasource caption='%s' inline='true' name='sqlproxy.%d' version='18.1'>\n", caption, index)
		fmt.Fprintf(&b, "      <repository-location derived-from='https://tableau.example.com/t/source-site/datasources/%[1]s?rev=1.0' id='%[1]s' path='/t/source-site/datasources' revision='1.0' site='source-site' />\n", reference.contentURL)
		fmt.Fprintf(&b, "      <connection channel='https' class='sqlproxy' dbname='%s' directory='dataserver' port='443' server='tableau.example.com'>\n      </connection>\n    </datasource>\n", reference.contentURL)
	}
	b.WriteString("  </datasources>\n</workbook>\n")
	return b.String()
}

// datasourceFilter parses one Tableau datasource list filter of the form
// field:operator:value or field:in:[a,b].
func datasourceFilter(t *testing.T, request *http.Request) (string, []string) {
	t.Helper()
	filter := request.URL.Query().Get("filter")
	field, rest, ok := strings.Cut(filter, ":")
	operator, value, ok2 := strings.Cut(rest, ":")
	if !ok || !ok2 {
		t.Errorf("datasource list filter = %q, want one exact field filter", filter)
		return "", nil
	}
	switch operator {
	case "eq":
		return field, []string{value}
	case "in":
		return field, strings.Split(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"), ",")
	}
	t.Errorf("datasource list filter operator = %q in %q", operator, filter)
	return "", nil
}

func sameSet(got, want []string) bool {
	got, want = slices.Clone(got), slices.Clone(want)
	slices.Sort(got)
	slices.Sort(want)
	return slices.Equal(got, want)
}

func unmappedOptions(t *testing.T, serverURL string, client *http.Client) app.Options {
	t.Helper()
	configPath := writePhaseOneConfigWithSite(t, serverURL, "target-site")
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	return withSiteMutationConsent(t, app.Options{ConfigPath: configPath, HTTPClient: client, JobDirectory: t.TempDir(), Now: func() time.Time { return time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC) }, CorrelationID: func() string { return "e2e-correlation" }}, true)
}

// publishFixture serves the requests every workbook publish makes before and
// around its upload. Datasource list requests go to listDatasources.
func publishFixture(t *testing.T, publish http.HandlerFunc, listDatasources func(field string, values []string) string, readDatasource func(luid string) string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		if request.URL.Query().Get("asJob") == "true" {
			t.Errorf("publish requested a Tableau job: %s", request.URL)
		}
		switch {
		case request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.Contains(request.URL.Path, "/users/"):
			t.Errorf("publish read the site role: %s", request.URL)
			_, _ = io.WriteString(writer, `<tsResponse><user id="user-1" name="publisher" siteRole="SiteAdministratorCreator"/></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="project-1" name="Finance"/><project id="project-2" name="Ops" parentProjectId="project-1"/></projects></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/workbooks":
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><workbooks/></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources":
			field, values := datasourceFilter(t, request)
			_, _ = io.WriteString(writer, listDatasources(field, values))
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/3.29/sites/site-1/datasources/"):
			_, _ = io.WriteString(writer, readDatasource(strings.TrimPrefix(request.URL.Path, "/api/3.29/sites/site-1/datasources/")))
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/workbooks/validateWorkbook":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"warnings":[]}`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/workbooks":
			publish(writer, request)
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL)
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}
}

func datasourcePage(items ...string) string {
	return fmt.Sprintf(`<tsResponse><pagination pageNumber="1" pageSize="%d" totalAvailable="%d"/><datasources>%s</datasources></tsResponse>`, max(len(items), 1), len(items), strings.Join(items, ""))
}

func rejectMissingDatasource(contentURL string) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(writer, `<tsResponse><error code="400011"><summary>Bad Request</summary><detail>There was a problem publishing the file 'Finance.twb'.. (0x9F4C1551 : com.tableausoftware.domain.exception.PublishingException: Datasource '%s' not found for workbook 'Finance'.)</detail></error></tsResponse>`, contentURL)
	}
}

func writeWorkbook(t *testing.T, contents string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "Finance.twb")
	if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestWorkbookPublishReportsEveryMissingDatasourceThroughCLI(t *testing.T) {
	var mu sync.Mutex
	var lookups []string
	listDatasources := func(field string, values []string) string {
		mu.Lock()
		lookups = append(lookups, field)
		mu.Unlock()
		switch field {
		case "contentUrl":
			if !sameSet(values, []string{"Alpha", "SalesMargin", "OpsEast", "Present"}) {
				t.Errorf("content URL lookup = %v, want every unmapped content URL and not the bound one", values)
			}
			return datasourcePage(`<datasource id="ds-present" name="Present" contentUrl="Present"><project id="project-1" name="Finance"/><owner id="user-1"/></datasource>`)
		case "name":
			if !sameSet(values, []string{"Alpha", "Sales & Margin"}) {
				t.Errorf("name lookup = %v, want the missing captions without the comma caption", values)
			}
			return datasourcePage(`<datasource id="ds-name" name="Sales &amp; Margin" contentUrl="SalesMargin_1" updatedAt="2026-01-02T03:04:05Z"><project id="project-2" name="Ops"/><owner id="user-9"/></datasource>`)
		}
		t.Errorf("unexpected datasource lookup field %q", field)
		return datasourcePage()
	}
	readDatasource := func(luid string) string {
		if luid != "ds-bound" {
			t.Errorf("unexpected datasource read %q", luid)
		}
		return `<tsResponse><datasource id="ds-bound" name="Bound" contentUrl="Bound_2"><project id="project-1" name="Finance"/><owner id="user-1"/></datasource></tsResponse>`
	}
	server := tableauFixtureServer(t, publishFixture(t, rejectMissingDatasource("Alpha"), listDatasources, readDatasource))
	defer server.Close()
	options := unmappedOptions(t, server.URL, server.Client())
	file := writeWorkbook(t, workbookWithDatasources(
		datasourceReference{"Alpha", "Alpha"},
		datasourceReference{"Sales & Margin", "SalesMargin"},
		datasourceReference{"Ops, East", "OpsEast"},
		datasourceReference{"Present", "Present"},
		datasourceReference{"Bound", "Bound"},
	))

	var out strings.Builder
	args := []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--datasource-binding", "Bound=ds-bound", "--json"}
	if exit := app.Run(context.Background(), args, &out, options); exit == 0 {
		t.Fatalf("publish succeeded:\n%s", out.String())
	}
	if !slices.Equal(lookups, []string{"contentUrl", "name"}) {
		t.Fatalf("datasource lookups = %v, want one content URL lookup then one name lookup", lookups)
	}
	var document struct {
		Error struct {
			ID               string `json:"id"`
			Summary          string `json:"summary"`
			Outcome          string `json:"outcome"`
			UpstreamCode     string `json:"upstream_code"`
			CorrectiveAction string `json:"corrective_action"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.String()), &document); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	failure := document.Error
	if failure.ID != "workbook.publish.datasource_unmapped" || failure.Outcome != "not_applied" || failure.UpstreamCode != "400011" {
		t.Fatalf("error = %#v", failure)
	}
	if failure.Summary != "Your workbook contains 3 data source(s) that do not map to a published data source on this Tableau Site." {
		t.Fatalf("summary = %q", failure.Summary)
	}
	action := failure.CorrectiveAction
	for _, want := range []string{
		"- Alpha (content URL Alpha):\n  No candidates found.",
		"- Sales & Margin (content URL SalesMargin):\n  - \"Sales & Margin\", content URL SalesMargin_1, LUID ds-name, project \"Ops\" (LUID project-2), owner user-9, updated 2026-01-02T03:04:05Z. Remap: ",
		"--datasource-binding Bound=ds-bound --datasource-binding SalesMargin=ds-name",
		"- Ops, East (content URL OpsEast):\n  Not searched by name because Tableau name filters cannot contain a comma.",
	} {
		if !strings.Contains(action, want) {
			t.Fatalf("corrective action missing %q:\n%s", want, action)
		}
	}
	for _, unwanted := range []string{"Present (content URL", "Bound (content URL", "_<digits>"} {
		if strings.Contains(action, unwanted) {
			t.Fatalf("corrective action contains %q:\n%s", unwanted, action)
		}
	}
}

func TestWorkbookPublishReturnsTableauErrorWhenEveryDatasourceExists(t *testing.T) {
	var mu sync.Mutex
	var lookups []string
	listDatasources := func(field string, values []string) string {
		mu.Lock()
		lookups = append(lookups, field)
		mu.Unlock()
		if field != "contentUrl" || !sameSet(values, []string{"Alpha", "Beta"}) {
			t.Errorf("datasource lookup = %s %v", field, values)
		}
		return datasourcePage(
			`<datasource id="ds-alpha" name="Alpha" contentUrl="Alpha"><project id="project-1" name="Finance"/><owner id="user-1"/></datasource>`,
			`<datasource id="ds-beta" name="Beta" contentUrl="Beta"><project id="project-1" name="Finance"/><owner id="user-1"/></datasource>`,
		)
	}
	server := tableauFixtureServer(t, publishFixture(t, rejectMissingDatasource("Alpha"), listDatasources, func(luid string) string {
		t.Errorf("unexpected datasource read %q", luid)
		return ""
	}))
	defer server.Close()
	options := unmappedOptions(t, server.URL, server.Client())
	file := writeWorkbook(t, workbookWithDatasources(datasourceReference{"Alpha", "Alpha"}, datasourceReference{"Beta", "Beta"}))

	var out strings.Builder
	args := []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1"}
	if exit := app.Run(context.Background(), args, &out, options); exit == 0 {
		t.Fatalf("publish succeeded:\n%s", out.String())
	}
	if !slices.Equal(lookups, []string{"contentUrl"}) {
		t.Fatalf("datasource lookups = %v, want one content URL lookup", lookups)
	}
	if strings.Contains(out.String(), "datasource_unmapped") || strings.Contains(out.String(), "do not map") || !strings.Contains(out.String(), "400011") || !strings.Contains(out.String(), "Datasource 'Alpha' not found") {
		t.Fatalf("want Tableau's own rejection:\n%s", out.String())
	}
}

func TestWorkbookPublishPreviewMakesNoDatasourceRequests(t *testing.T) {
	server := tableauFixtureServer(t, publishFixture(t, func(http.ResponseWriter, *http.Request) {
		t.Error("preview uploaded the workbook")
	}, func(field string, values []string) string {
		t.Errorf("preview listed datasources by %s %v", field, values)
		return datasourcePage()
	}, func(luid string) string {
		t.Errorf("preview read datasource %q", luid)
		return ""
	}))
	defer server.Close()
	options := unmappedOptions(t, server.URL, server.Client())
	file := writeWorkbook(t, workbookWithDatasources(datasourceReference{"Alpha", "Alpha"}, datasourceReference{"Sales & Margin", "SalesMargin"}))

	var out strings.Builder
	args := []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--preview", "--full"}
	if exit := app.Run(context.Background(), args, &out, options); exit != 0 {
		t.Fatalf("preview exit = %d:\n%s", exit, out.String())
	}
	if !strings.Contains(out.String(), "datasource_bindings[2]{datasource,content_url,origin,target_content_url,target_luid}:") || !strings.Contains(out.String(), "Sales & Margin,SalesMargin,workbook,SalesMargin,\"\"") {
		t.Fatalf("preview missing the workbook datasource list:\n%s", out.String())
	}
	for _, unwanted := range []string{"datasource_candidates", "likely fail", "candidate search"} {
		if strings.Contains(out.String(), unwanted) {
			t.Fatalf("preview contains %q:\n%s", unwanted, out.String())
		}
	}
}

func TestPublishNeverUsesTableauJobsForAdministrators(t *testing.T) {
	created := `<datasource id="ds-new" name="Sales" contentUrl="SalesMargin"><project id="project-1" name="Finance"/><owner id="user-1"/></datasource>`
	server := tableauFixtureServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		if request.URL.Query().Get("asJob") == "true" {
			t.Errorf("publish requested a Tableau job: %s", request.URL)
		}
		switch {
		case request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.Contains(request.URL.Path, "/users/"):
			t.Errorf("publish read the site role: %s", request.URL)
			_, _ = io.WriteString(writer, `<tsResponse><user id="user-1" name="publisher" siteRole="SiteAdministratorCreator"/></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Finance" topLevelProject="true"/></projects></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/workbooks":
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><workbooks/></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources/ds-new":
			_, _ = io.WriteString(writer, `<tsResponse>`+created+`</tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources":
			_, _ = io.WriteString(writer, datasourcePage())
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/workbooks/validateWorkbook":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"warnings":[]}`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/workbooks":
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-1" name="Finance"><project id="project-1"/></workbook></tsResponse>`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/datasources":
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse>`+created+`</tsResponse>`)
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL)
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	options := unmappedOptions(t, server.URL, server.Client())
	workbook := writeWorkbook(t, workbookWithDatasources())
	datasource := filepath.Join(t.TempDir(), "Sales.tds")
	if err := os.WriteFile(datasource, []byte(`<datasource><repository-location derived-from='https://tableau.example.com/t/source-site/datasources/SalesMargin?rev=1.0' id='SalesMargin' path='/t/source-site/datasources' revision='1.0' site='source-site' /><connection class='postgres'/></datasource>`), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name, want string
		args       []string
	}{
		{"workbook", "workbook_luid: wb-1", []string{"content", "workbook", "publish", "--file", workbook, "--environment", "production", "--project-id", "project-1"}},
		{"datasource", "datasource_luid: ds-new", []string{"content", "datasource", "publish", "--file", datasource, "--environment", "production", "--project-id", "project-1", "--create"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out strings.Builder
			if exit := app.Run(context.Background(), test.args, &out, options); exit != 0 {
				t.Fatalf("exit = %d:\n%s", exit, out.String())
			}
			if !strings.Contains(out.String(), test.want) || !strings.Contains(out.String(), "verification: confirmed") || strings.Contains(out.String(), "tableau_job_id") {
				t.Fatalf("want a confirmed synchronous publish:\n%s", out.String())
			}
		})
	}
}

func TestWorkbookBatchPublishRunsEveryUploadAtOnce(t *testing.T) {
	const items = 10
	var mu sync.Mutex
	active, peak, uploads := 0, 0, 0
	release := make(chan struct{})
	var releaseOnce sync.Once
	server := tableauFixtureServer(t, publishFixture(t, func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		name := regexp.MustCompile(`<workbook name="([^"]+)"`).FindStringSubmatch(string(body))
		mu.Lock()
		active++
		uploads++
		number := uploads
		peak = max(peak, active)
		if active == items {
			releaseOnce.Do(func() { close(release) })
		}
		mu.Unlock()
		select {
		case <-release:
		case <-time.After(2 * time.Second):
			// Not every upload overlapped; let the rest finish quickly.
			releaseOnce.Do(func() { close(release) })
		}
		mu.Lock()
		active--
		mu.Unlock()
		if len(name) != 2 {
			t.Errorf("upload payload has no workbook name")
			http.Error(writer, "missing name", http.StatusBadRequest)
			return
		}
		writer.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(writer, `<tsResponse><workbook id="wb-%d" name="%s"><project id="project-1"/></workbook></tsResponse>`, number, name[1])
	}, func(field string, values []string) string {
		t.Errorf("batch publish listed datasources by %s %v", field, values)
		return datasourcePage()
	}, func(luid string) string {
		t.Errorf("batch publish read datasource %q", luid)
		return ""
	}))
	defer server.Close()
	options := unmappedOptions(t, server.URL, server.Client())
	workbook := writeWorkbook(t, workbookWithDatasources())
	type row struct {
		File      string `json:"file"`
		ProjectID string `json:"project-id"`
		Name      string `json:"name"`
	}
	var rows []row
	for index := 1; index <= items; index++ {
		rows = append(rows, row{File: workbook, ProjectID: "project-1", Name: fmt.Sprintf("Finance %02d", index)})
	}
	data, err := json.Marshal(map[string]any{"items": rows})
	if err != nil {
		t.Fatal(err)
	}
	batchFile := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(batchFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if exit := app.Run(context.Background(), []string{"content", "workbook", "publish", "--batch-file", batchFile, "--environment", "production", "--json"}, &out, options); exit != 0 {
		t.Fatalf("exit = %d:\n%s", exit, out.String())
	}
	if peak != items {
		t.Fatalf("peak concurrent uploads = %d, want %d", peak, items)
	}
	var document struct {
		Items []struct {
			Status string          `json:"status"`
			Result json.RawMessage `json:"result"`
		} `json:"items"`
		Help []string `json:"help"`
	}
	if err := json.Unmarshal([]byte(out.String()), &document); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	if len(document.Items) != len(rows) {
		t.Fatalf("items = %d, want %d:\n%s", len(document.Items), len(rows), out.String())
	}
	for index, item := range document.Items {
		if item.Status != "succeeded" || !strings.Contains(string(item.Result), `"`+rows[index].Name+`"`) {
			t.Fatalf("item %d = %s %s, want %q in selection order", index, item.Status, item.Result, rows[index].Name)
		}
	}
	if help := strings.Join(document.Help, " "); !strings.Contains(help, "in parallel") || !strings.Contains(help, "selection order") {
		t.Fatalf("help = %q", help)
	}
}

func TestDatasourceListFiltersNamesContainingAmpersands(t *testing.T) {
	var mu sync.Mutex
	var filters []string
	server := tableauFixtureServer(t, publishFixture(t, func(http.ResponseWriter, *http.Request) {
		t.Error("list uploaded a workbook")
	}, func(field string, values []string) string {
		mu.Lock()
		filters = append(filters, field+"="+strings.Join(values, "|"))
		mu.Unlock()
		return datasourcePage(`<datasource id="ds-name" name="Sales &amp; Margin" contentUrl="SalesMargin"><project id="project-1" name="Finance"/><owner id="user-1"/></datasource>`)
	}, func(luid string) string {
		t.Errorf("list read datasource %q", luid)
		return ""
	}))
	defer server.Close()
	options := unmappedOptions(t, server.URL, server.Client())

	var out strings.Builder
	if exit := app.Run(context.Background(), []string{"content", "datasource", "list", "--name", "Sales & Margin", "--environment", "production"}, &out, options); exit != 0 {
		t.Fatalf("exit = %d:\n%s", exit, out.String())
	}
	if !slices.Equal(filters, []string{"name=Sales & Margin"}) {
		t.Fatalf("filters = %v, want one exact name filter", filters)
	}
	if !strings.Contains(out.String(), "ds-name") {
		t.Fatalf("list output missing the match:\n%s", out.String())
	}
}
