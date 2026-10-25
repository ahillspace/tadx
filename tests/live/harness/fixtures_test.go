package harness

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The integration test uses a selected local SDK, never a network install.
func TestGeneratedNativePackHasExactHyperRows(t *testing.T) {
	python := os.Getenv("TADX_TEST_HYPER_PYTHON")
	if python == "" {
		t.Skip("set TADX_TEST_HYPER_PYTHON to a Python executable with tableauhyperapi installed")
	}
	if _, err := os.Stat(python); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TADX_LIVECLI_PYTHON", python)
	caller := t.TempDir()
	t.Chdir(caller)
	artifacts, bindings, err := GenerateFixtures(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"datasource", "workbook"} {
		for _, suffix := range []string{"", "-b"} {
			key := kind + "-native" + suffix
			content, err := os.ReadFile(artifacts[key])
			if err != nil {
				t.Fatal(err)
			}
			if err := CheckNative(t.Context(), kind, content, bindings[key]); err != nil {
				t.Fatalf("%s: %v", key, err)
			}
			if kind == "workbook" {
				assertGeneratedWorkbookReferences(t, content)
			}
		}
	}
	aData, err := os.ReadFile(artifacts["datasource-native"])
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckNative(t.Context(), "datasource", aData, bindings["datasource-native-b"]); err == nil {
		t.Fatal("variant A accepted by authored B row oracle")
	}
	for _, key := range []string{"datasource-hyper", "datasource-hyper-b"} {
		content, err := os.ReadFile(artifacts[key])
		if err != nil {
			t.Fatal(err)
		}
		if err := CheckNative(t.Context(), "datasource", content, bindings[key]); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
	}
	if err := CheckNative(t.Context(), "datasource", aData, bindings["datasource-append-result"]); err == nil {
		t.Fatal("unappended baseline accepted by exact append-result oracle")
	}
	entries, err := os.ReadDir(caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("Hyper fixture generation or native verification leaked runtime files into the caller directory")
	}
}

func TestGeneratedAppendOracleUsesExactHyperFixtureTruth(t *testing.T) {
	python := os.Getenv("TADX_TEST_HYPER_PYTHON")
	if python == "" {
		t.Skip("set TADX_TEST_HYPER_PYTHON to a Python executable with tableauhyperapi installed")
	}
	if _, err := os.Stat(python); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TADX_LIVECLI_PYTHON", python)
	artifacts, bindings, err := GenerateFixtures(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := bindings["datasource-native-hyper"]
	b := bindings["datasource-native-hyper-b"]
	appendResult := bindings["datasource-append-result"]
	if _, ok := appendResult.Expected["columns"]; ok {
		t.Fatal("append result inherited packaged datasource XML columns instead of Hyper fixture truth")
	}
	if _, ok := appendResult.Expected["worksheets"]; ok {
		t.Fatal("append result inherited packaged datasource XML worksheets instead of Hyper fixture truth")
	}
	if !reflect.DeepEqual(withoutRows(appendResult.Expected), withoutRows(a.Expected)) {
		t.Fatal("append result metadata differs from the standalone Hyper fixture expectations")
	}
	aRows, ok := a.Expected["rows"].([]any)
	if !ok {
		t.Fatal("A Hyper fixture is missing its authored rows")
	}
	bRows, ok := b.Expected["rows"].([]any)
	if !ok {
		t.Fatal("B Hyper fixture is missing its authored rows")
	}
	wantRows := append(slices.Clone(aRows), bRows...)
	if !reflect.DeepEqual(appendResult.Expected["rows"], wantRows) {
		t.Fatal("append result does not contain the exact authored A plus B row multiset")
	}
	if appendResult.Params["hyper_table"] != a.Params["hyper_table"] {
		t.Fatal("append result does not retain the Hyper fixture table selector")
	}

	directory := t.TempDir()
	appendPath := filepath.Join(directory, "append.hyper")
	if err := combineHyperFixtures(t.Context(), python, appendPath, artifacts["datasource-native-hyper"], artifacts["datasource-native-hyper-b"]); err != nil {
		t.Fatal(err)
	}
	appendBinding := appendResult
	appendBinding.Path = appendPath
	if err := CheckNative(t.Context(), "datasource", mustReadFile(t, appendPath), appendBinding); err != nil {
		t.Fatalf("exact authored A plus B Hyper rows were rejected: %v", err)
	}
	for _, scenario := range []struct {
		name  string
		files []string
	}{
		{name: "missing rows", files: []string{artifacts["datasource-native-hyper"]}},
		{name: "wrong rows", files: []string{artifacts["datasource-native-hyper-b"]}},
		{name: "duplicate rows", files: []string{artifacts["datasource-native-hyper"], artifacts["datasource-native-hyper-b"], artifacts["datasource-native-hyper"]}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			actualPath := filepath.Join(t.TempDir(), "actual.hyper")
			if err := combineHyperFixtures(t.Context(), python, actualPath, scenario.files...); err != nil {
				t.Fatal(err)
			}
			actualBinding := appendResult
			actualBinding.Path = actualPath
			if err := CheckNative(t.Context(), "datasource", mustReadFile(t, actualPath), actualBinding); err == nil {
				t.Fatal("append row oracle accepted a non-union Hyper extract")
			}
		})
	}
}

func withoutRows(expected map[string]any) map[string]any {
	result := maps.Clone(expected)
	delete(result, "rows")
	return result
}

func combineHyperFixtures(ctx context.Context, python, output string, inputs ...string) error {
	const script = `import sys
from tableauhyperapi import Connection, CreateMode, HyperProcess, Inserter, SqlType, TableDefinition, TableName, Telemetry
table = TableDefinition(TableName("Extract", "Extract"), [
    TableDefinition.Column("ID", SqlType.big_int()),
    TableDefinition.Column("Order Date", SqlType.date()),
    TableDefinition.Column("Region", SqlType.text()),
    TableDefinition.Column("Sales", SqlType.double()),
])
with HyperProcess(Telemetry.DO_NOT_SEND_USAGE_DATA_TO_TABLEAU, parameters={"log_config": ""}) as hyper:
    with Connection(hyper.endpoint, sys.argv[1], CreateMode.CREATE_AND_REPLACE) as target:
        target.catalog.create_schema("Extract")
        target.catalog.create_table(table)
        with Inserter(target, table) as inserter:
            for path in sys.argv[2:]:
                with Connection(hyper.endpoint, path) as source:
                    inserter.add_rows(source.execute_list_query('SELECT * FROM "Extract"."Extract"'))
            inserter.execute()
`
	args := append([]string{"-c", script, output}, inputs...)
	command := exec.CommandContext(ctx, python, args...)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("combine Hyper fixture rows: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func assertGeneratedWorkbookReferences(t *testing.T, data []byte) {
	t.Helper()
	evidence, err := ValidateNative("workbook", Native{Bytes: data, Filename: "fixture.twbx"})
	if err != nil {
		t.Fatal(err)
	}
	type named struct {
		Name string `xml:"name,attr"`
	}
	var workbook struct {
		Datasources []struct {
			Name       string `xml:"name,attr"`
			Connection struct {
				Database string `xml:"dbname,attr"`
			} `xml:"connection"`
			Extract struct {
				Connection struct {
					Database string `xml:"dbname,attr"`
				} `xml:"connection"`
			} `xml:"extract"`
			Columns []named `xml:"column"`
		} `xml:"datasources>datasource"`
		Worksheets []struct {
			Name  string `xml:"name,attr"`
			Table struct {
				View struct {
					Datasources  []named `xml:"datasources>datasource"`
					Dependencies []struct {
						Datasource string `xml:"datasource,attr"`
						Instances  []struct {
							Name   string `xml:"name,attr"`
							Column string `xml:"column,attr"`
						} `xml:"column-instance"`
					} `xml:"datasource-dependencies"`
				} `xml:"view"`
				Rows    string `xml:"rows"`
				Columns string `xml:"cols"`
			} `xml:"table"`
		} `xml:"worksheets>worksheet"`
		Windows []named `xml:"windows>window"`
	}
	if err := xml.Unmarshal(evidence.Payloads[evidence.Document], &workbook); err != nil {
		t.Fatal(err)
	}
	if len(workbook.Datasources) != 1 || len(workbook.Worksheets) != 1 || len(workbook.Windows) != 1 {
		t.Fatal("generated workbook must have one datasource, worksheet, and worksheet window")
	}
	datasource := workbook.Datasources[0]
	for _, database := range []string{datasource.Connection.Database, datasource.Extract.Connection.Database} {
		if _, ok := evidence.Payloads[database]; !ok {
			t.Fatalf("workbook connection references unpackaged extract %q", database)
		}
	}
	columns := map[string]bool{}
	for _, column := range datasource.Columns {
		columns[column.Name] = true
	}
	sheet := workbook.Worksheets[0]
	if workbook.Windows[0].Name != sheet.Name {
		t.Fatal("workbook window references an absent worksheet")
	}
	if len(sheet.Table.View.Datasources) != 1 || sheet.Table.View.Datasources[0].Name != datasource.Name || len(sheet.Table.View.Dependencies) != 1 || sheet.Table.View.Dependencies[0].Datasource != datasource.Name {
		t.Fatal("worksheet references a missing datasource")
	}
	instances := map[string]bool{}
	for _, instance := range sheet.Table.View.Dependencies[0].Instances {
		if !columns[instance.Column] {
			t.Fatalf("worksheet instance references absent source column %q", instance.Column)
		}
		instances["["+datasource.Name+"]."+instance.Name] = true
	}
	if !instances[sheet.Table.Rows] || !instances[sheet.Table.Columns] {
		t.Fatal("worksheet axes reference missing column instances")
	}
}

func TestEphemeralContentNamesUseBaselineButAdvancedVariantsRemainExplicit(t *testing.T) {
	baseline := []byte(`<datasource name="authored"><column name="[Sales]"/></datasource>`)
	filename := writeNativeTestFile(t, "fixture.tds", baseline)
	projects, publishes := 0, 0
	projectRows := []map[string]any{}
	sources := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/3.29/sites/site/projects" {
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(map[string]any{"projects": map[string]any{"project": projectRows}, "pagination": map[string]any{"totalAvailable": len(projectRows)}})
				return
			}
			projects++
			var payload struct {
				Project struct {
					Name   string `json:"name"`
					Parent string `json:"parentProjectId"`
				} `json:"project"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			row := map[string]any{"id": fmt.Sprintf("p%d", projects), "name": payload.Project.Name, "parentProjectId": payload.Project.Parent}
			projectRows = append(projectRows, row)
			_ = json.NewEncoder(w).Encode(map[string]any{"project": row})
			return
		}
		if r.Method == http.MethodGet {
			row, ok := sources[strings.TrimPrefix(r.URL.Path, "/api/3.29/sites/site/datasources/")]
			if !ok {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"datasource": row})
			return
		}
		publishes++
		parts, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		request, err := parts.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		document, err := io.ReadAll(request)
		if err != nil {
			t.Error(err)
			return
		}
		var payload struct {
			Datasource struct {
				Name    string `xml:"name,attr"`
				Project struct {
					ID string `xml:"id,attr"`
				} `xml:"project"`
			} `xml:"datasource"`
		}
		if err := xml.Unmarshal(document, &payload); err != nil {
			t.Error(err)
			return
		}
		native, err := parts.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		content, err := io.ReadAll(native)
		if err != nil {
			t.Error(err)
			return
		}
		if string(content) != string(baseline) {
			t.Error("ephemeral fixture used another payload")
		}
		row := map[string]any{"id": fmt.Sprintf("ds%d", publishes), "name": payload.Datasource.Name, "project": map[string]any{"id": payload.Datasource.Project.ID}}
		sources[fmt.Sprintf("ds%d", publishes)] = row
		_ = json.NewEncoder(w).Encode(map[string]any{"datasource": row})
	}))
	defer server.Close()
	journal, err := NewJournal(filepath.Join(t.TempDir(), "journal.json"), "run", "target", "tadx-live-fixtures")
	if err != nil {
		t.Fatal(err)
	}
	c := &Case{Root: "root-project", NamePrefix: "tadx-live-fixtures", Journal: journal, API: &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}, Profile: Profile{Artifacts: map[string]string{"datasource-native": filename}}}
	base, err := c.Content(t.Context(), "datasource", "base")
	if err != nil {
		t.Fatal(err)
	}
	sentinel, err := c.Content(t.Context(), "datasource", "sentinel")
	if err != nil {
		t.Fatal(err)
	}
	if base.ID == sentinel.ID || base.Name == sentinel.Name {
		t.Fatal("ephemeral fixture namespaces share remote identity")
	}
	first, err := c.Seed(t.Context(), "datasource", "source")
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Seed(t.Context(), "datasource", "source")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.Name == second.Name {
		t.Fatal("fresh content seeds reused remote identity")
	}
	before := publishes
	if _, err := c.Content(t.Context(), "datasource", "dependencies"); ErrorStatus(err) != BlockedStatus {
		t.Fatalf("advanced variant silently used baseline: %v", err)
	}
	if publishes != before {
		t.Fatal("missing advanced fixture reached remote publisher")
	}
}

func TestFixtureVisibilityWaitsAfterDurableReceiptWithoutRepeatingCreate(t *testing.T) {
	journal := testJournal(t)
	creates, reads := 0, 0
	name := "tadx-live-safety-visible"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			creates++
			fmt.Fprintf(w, `{"group":{"id":"announced-group","name":%q}}`, name)
			return
		}
		reads++
		saved, err := LoadJournal(journal.path, journal.Target)
		if err != nil {
			t.Error(err)
		} else if len(saved.Entries) != 1 || saved.Entries[0].Object == nil || saved.Entries[0].Object.ID != "announced-group" {
			t.Error("independent observation began before confirmed identity was durable")
		}
		if reads == 1 {
			fmt.Fprint(w, `{"groups":{"group":[]},"pagination":{"totalAvailable":"0"}}`)
			return
		}
		visibleName := name
		if reads == 2 {
			visibleName = "stale-name"
		}
		fmt.Fprintf(w, `{"groups":{"group":[{"id":"announced-group","name":%q}]},"pagination":{"totalAvailable":"1"}}`, visibleName)
	}))
	defer server.Close()
	c := &Case{NamePrefix: "tadx-live-safety", Journal: journal, API: observationClient(server), Profile: Profile{Budgets: Budgets{ObservationSeconds: 3}}}
	group, err := c.Seed(t.Context(), "group", "visible")
	if err != nil {
		t.Fatal(err)
	}
	if group.ID != "announced-group" || group.Name != name || creates != 1 || reads != 3 {
		t.Fatalf("fixture readiness retried mutation or accepted stale fields: %#v creates=%d reads=%d", group, creates, reads)
	}
}

func TestFixtureVisibilityTimeoutRetainsAnnouncedIdentity(t *testing.T) {
	journal := testJournal(t)
	creates := 0
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			creates++
			fmt.Fprint(w, `{"group":{"id":"announced-group"}}`)
			return
		}
		cancel()
		fmt.Fprint(w, `{"groups":{"group":[]},"pagination":{"totalAvailable":"0"}}`)
	}))
	defer server.Close()
	c := &Case{NamePrefix: "tadx-live-safety", Journal: journal, API: observationClient(server), Profile: Profile{Budgets: Budgets{ObservationSeconds: 1}}}
	receipt, err := c.Seed(ctx, "group", "invisible")
	if err == nil {
		t.Fatal("fixture passed despite missing authoritative visibility")
	}
	if receipt.ID != "announced-group" || receipt.Name != "" || creates != 1 {
		t.Fatal("observation timeout discarded known identity or retried mutation")
	}
	saved, err := LoadJournal(journal.path, journal.Target)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Entries) != 1 || saved.Entries[0].Object == nil || saved.Entries[0].Object.ID != "announced-group" || saved.Entries[0].Done {
		t.Fatal("timed-out fixture identity was not recoverable for cleanup")
	}
}

func TestSparseFixtureReceiptIsDurableBeforeExactHydration(t *testing.T) {
	j := testJournal(t)
	name := "tadx-live-safety-sparse"
	reads, creates := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			creates++
			fmt.Fprint(w, `{"group":{"id":"announced-group"}}`)
			return
		}
		reads++
		saved, err := LoadJournal(j.path, j.Target)
		if err != nil {
			t.Error(err)
		} else if len(saved.Entries) != 1 || saved.Entries[0].Object == nil || saved.Entries[0].Object.ID != "announced-group" || saved.Entries[0].Object.Name != "" {
			t.Error("hydration began before sparse authoritative receipt was saved unchanged")
		}
		fmt.Fprintf(w, `{"groups":{"group":[{"id":"announced-group","name":%q}]},"pagination":{"totalAvailable":"1"}}`, name)
	}))
	defer server.Close()
	c := &Case{NamePrefix: "tadx-live-safety", Journal: j, API: observationClient(server), Profile: Profile{Budgets: Budgets{ObservationSeconds: 1}}}
	group, err := c.Seed(t.Context(), "group", "sparse")
	if err != nil {
		t.Fatal(err)
	}
	if group.ID != "announced-group" || group.Name != name || creates != 1 || reads != 1 {
		t.Fatal("sparse fixture receipt was not independently hydrated by exact returned ID")
	}
	saved, err := LoadJournal(j.path, j.Target)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Entries[0].Object.Name != name {
		t.Fatal("independent hydration was not durable")
	}
}
