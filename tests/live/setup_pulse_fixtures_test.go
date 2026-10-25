package live

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func TestPulseNativeInputValidatesAuthoredFieldsAndNeverNeedsRows(t *testing.T) {
	binding := validPulseNativeInput(t)
	if err := validatePulseNativeInput(binding); err != nil {
		t.Fatal(err)
	}
	if _, exists := binding.Expected["rows"]; exists {
		t.Fatal("Pulse native input unexpectedly requires a full row oracle")
	}
}

func TestPulseNativeInputMatchesPlainVDSNamesToExactNativeMarkers(t *testing.T) {
	input := validPulseNativeInput(t)
	fields := input.Expected["pulse_fields"].([]any)
	for _, raw := range fields {
		field := raw.(map[string]any)
		field["name"] = strings.Trim(nativeText(field["name"]), "[]")
	}
	input.Params["measure_name"] = "Sales"
	input.Params["date_name"] = "Order Date"
	input.Params["dimension_name"] = "Product Category"
	if err := validatePulseNativeInput(input); err != nil {
		t.Fatalf("plain VDS names did not match exact native markers: %v", err)
	}

	columns := []string{"Sales", "Order Date", "Product Category"}
	input.Expected["columns"] = columns
	if err := validatePulseNativeInput(input); err != nil {
		t.Fatalf("exact plain names were rejected: %v", err)
	}

	for _, invalid := range []string{"sales", "Sales extra", "[Sales]]", "[[Sales]", "[sales]"} {
		t.Run(invalid, func(t *testing.T) {
			bad := validPulseNativeInput(t)
			bad.Expected["columns"] = []string{invalid, "[Order Date]", "[Product Category]"}
			bad.Expected["pulse_fields"].([]any)[0].(map[string]any)["name"] = "Sales"
			bad.Params["measure_name"] = "Sales"
			if err := validatePulseNativeInput(bad); err == nil {
				t.Fatalf("invalid native marker %q was accepted", invalid)
			}
		})
	}
}

func TestPulseNativeInputRejectsMissingOrDuplicateFieldFacts(t *testing.T) {
	for name, mutate := range map[string]func(harness.Binding) harness.Binding{
		"missing selected field": func(binding harness.Binding) harness.Binding {
			binding.Params["measure_name"] = "[Missing]"
			return binding
		},
		"duplicate field": func(binding harness.Binding) harness.Binding {
			binding.Expected["pulse_fields"] = append(binding.Expected["pulse_fields"].([]any), map[string]any{"name": "[Sales]", "type": "integer", "role": "measure"})
			return binding
		},
		"missing role": func(binding harness.Binding) harness.Binding {
			binding.Expected["pulse_fields"].([]any)[0].(map[string]any)["role"] = ""
			return binding
		},
		"fractional extract count": func(binding harness.Binding) harness.Binding {
			binding.Expected["extract_count"] = 1.5
			return binding
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validatePulseNativeInput(mutate(validPulseNativeInput(t))); err == nil {
				t.Fatal("invalid authored Pulse fields passed validation")
			}
		})
	}
}

func TestPulseNativeMappingUsesTargetProviderIDsAndRejectsIneligibleFields(t *testing.T) {
	input := validPulseNativeInput(t)
	fields := []harness.PulseSchemaField{
		{ID: "[Sales]", Name: "[Sales]", Caption: "Sales", Role: "measure", DataType: "integer", ColumnClass: "column"},
		{ID: "[Order Date]", Name: "[Order Date]", Caption: "Order Date", Role: "dimension", DataType: "date", ColumnClass: "column"},
		{ID: "[Product Category]", Name: "[Product Category]", Caption: "Product Category", Role: "dimension", DataType: "string", ColumnClass: "column"},
	}
	got, err := resolvePulseFields(input, fields)
	if err != nil {
		t.Fatal(err)
	}
	if got["measure_field"] != "[Sales]" || got["date_field"] != "[Order Date]" || got["dimension_field"] != "[Product Category]" || got["category"] != "Electronics" {
		t.Fatalf("target mapping = %#v", got)
	}
	bad := append([]harness.PulseSchemaField(nil), fields...)
	bad[0].Hidden = true
	if _, err := resolvePulseFields(input, bad); err == nil {
		t.Fatal("hidden measure passed mapping")
	}
	bad = append([]harness.PulseSchemaField(nil), fields...)
	bad[0].Role = "dimension"
	if _, err := resolvePulseFields(input, bad); err == nil {
		t.Fatal("wrong-role measure passed mapping")
	}
	bad = append(bad, fields[0])
	if _, err := resolvePulseFields(input, bad); err == nil {
		t.Fatal("ambiguous field mapping passed")
	}
	bad = append([]harness.PulseSchemaField(nil), fields...)
	bad[0].ColumnClass = "calculation"
	if _, err := resolvePulseFields(input, bad); err == nil {
		t.Fatal("calculated field passed native physical field selection")
	}
	bad = append([]harness.PulseSchemaField(nil), fields...)
	bad[0].Excluded = true
	if _, err := resolvePulseFields(input, bad); err == nil {
		t.Fatal("explicitly excluded field passed selection")
	}
	bad = append([]harness.PulseSchemaField(nil), fields...)
	bad[0].Internal = true
	if _, err := resolvePulseFields(input, bad); err == nil {
		t.Fatal("explicitly internal field passed selection")
	}
	bad = append([]harness.PulseSchemaField(nil), fields...)
	bad[0].DataType = "real"
	if _, err := resolvePulseFields(input, bad); err == nil {
		t.Fatal("wrong logical data type passed selection")
	}
	bad = append([]harness.PulseSchemaField(nil), fields...)
	bad[0].ID, bad[0].Name = "other-provider-name", "other-provider-name"
	if _, err := resolvePulseFields(input, bad); err == nil {
		t.Fatal("caption-only match substituted for the authored native field")
	}
	if err := pulseFieldsCompatible(got, map[string]string{"measure_field": "new-measure-id", "date_field": "[Order Date]", "dimension_field": "[Product Category]"}); err == nil {
		t.Fatal("incompatible migration datasource was silently remapped")
	}
}

func TestPulseReadinessDeadlinePreservesSafeLastCauseAndStatus(t *testing.T) {
	observation := harness.Inconclusive("read-only observation ended before its contract could be established")
	for _, test := range []struct {
		name, cause, want string
	}{
		{name: "missing field", cause: "target datasource schema is not yet ready for measure_name", want: "not yet ready for measure_name"},
		{name: "HTTP failure", cause: "HTTP", want: "POST /api/v1/vizql-data-service/read-metadata returned HTTP 503"},
		{name: "unsafe transport detail", cause: "transport error containing secret", want: "Pulse schema read failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var checkErr error
			if test.name == "HTTP failure" {
				checkErr = &harness.HTTPError{Method: "POST", Path: "/api/v1/vizql-data-service/read-metadata", Status: 503}
			} else if test.name == "unsafe transport detail" {
				checkErr = errors.New(test.cause)
			} else {
				checkErr = harness.Inconclusive(test.cause)
			}
			got := preservePulseReadinessCause(observation, checkErr)
			if harness.ErrorStatus(got) != harness.InconclusiveStatus || !strings.Contains(got.Error(), test.want) {
				t.Fatalf("preserved readiness error = %v", got)
			}
			if strings.Contains(got.Error(), "secret") {
				t.Fatal("readiness error exposed unsafe transport detail")
			}
		})
	}
}

func TestPulseReadinessDoesNotInventCauseWhenNoLastCheckError(t *testing.T) {
	observation := harness.Inconclusive("read-only observation ended before its contract could be established")
	if got := preservePulseReadinessCause(observation, nil); got != observation {
		t.Fatalf("error without last cause changed: %v", got)
	}
}

func TestPulseNativeSetupNeverRepeatsUncertainProjectCreation(t *testing.T) {
	input := validPulseNativeInput(t)
	input.Path = filepath.Join(t.TempDir(), "source.tdsx")
	if err := os.WriteFile(input.Path, pulseTestTDSX(t), 0600); err != nil {
		t.Fatal(err)
	}
	journal, err := harness.NewJournal(filepath.Join(t.TempDir(), "journal.json"), "setup-run", "target", "tadx-live-pulse-uncertain")
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.AddIntent("project", "tadx-live-pulse-uncertain-pulse-source-project", "root"); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	t.Cleanup(server.Close)
	c := &harness.Case{Root: "root", NamePrefix: journal.Prefix, Journal: journal, API: &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}, Profile: harness.Profile{Bindings: map[string]harness.Binding{pulseNativeBinding: input}}}
	if err := PreparePulseFixtures(t.Context(), c, false); harness.ErrorStatus(err) != harness.InconclusiveStatus {
		t.Fatalf("uncertain project creation was not retained: %v", err)
	}
	if requests != 0 {
		t.Fatal("setup submitted another project creation after an unresolved intent")
	}
}

func TestPulseNativeSelectionAddsOnlyDatasourceFixtureCapabilities(t *testing.T) {
	pulsePlan, err := Select(Selectors{IDs: []string{"live.pulse-definition.create"}})
	if err != nil {
		t.Fatal(err)
	}
	pulseSpecs, err := specsFor(pulsePlan)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "source.tdsx")
	if err := os.WriteFile(path, []byte("portable fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	p := harness.Profile{Bindings: map[string]harness.Binding{pulseNativeBinding: {Path: path}}}
	caps := profileFixtureCapabilities(pulseSpecs, p)
	for _, capability := range []string{"datasource.publish", "datasource.delete", "datasource.schema", "job.inspect"} {
		if !slices.Contains(caps, capability) {
			t.Errorf("Pulse fixture capability %q missing", capability)
		}
	}
	otherPlan, err := Select(Selectors{IDs: []string{"live.admin-label-category.list"}})
	if err != nil {
		t.Fatal(err)
	}
	otherSpecs, err := specsFor(otherPlan)
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range profileFixtureCapabilities(otherSpecs, p) {
		if capability == "datasource.publish" || capability == "datasource.schema" {
			t.Errorf("unrelated selection gained Pulse datasource capability %s", capability)
		}
	}
}

func TestPulseNativeSetupValidationPreservesAccountPrerequisites(t *testing.T) {
	plan, err := Select(Selectors{IDs: []string{"live.pulse-metric.follow"}})
	if err != nil {
		t.Fatal(err)
	}
	specs, err := specsFor(plan)
	if err != nil {
		t.Fatal(err)
	}
	p := harness.Profile{Bindings: map[string]harness.Binding{
		"pulse-native": validPulseNativeInput(t),
		"user_a":       {ID: "user-a"},
		"user_b":       {ID: "user-b"},
	}}
	if err := validateBindings(specs, p); err != nil {
		t.Fatalf("native setup should validate user bindings independently of derived datasource IDs: %v", err)
	}
	delete(p.Bindings, "user_b")
	if err := validateBindings(specs, p); err == nil {
		t.Fatal("native datasource input bypassed required user_b binding")
	}
}

func TestPulseNativeInputMustMatchRetainedManifestForReadiness(t *testing.T) {
	saved := harness.Profile{Bindings: map[string]harness.Binding{"pulse-native": {Path: "fixture.tdsx", Params: map[string]string{"category": "Electronics"}}}}
	current := saved
	if err := verifyPulseNativeInputProfile(saved, current); err != nil {
		t.Fatal(err)
	}
	current.Bindings = map[string]harness.Binding{"pulse-native": {Path: "other.tdsx", Params: map[string]string{"category": "Electronics"}}}
	if err := verifyPulseNativeInputProfile(saved, current); harness.ErrorStatus(err) != harness.BlockedStatus {
		t.Fatalf("changed setup input was not rejected: %v", err)
	}
	if err := verifyPulseNativeInputProfile(saved, harness.Profile{}); harness.ErrorStatus(err) != harness.BlockedStatus {
		t.Fatalf("missing setup input was not rejected: %v", err)
	}
}

func TestVerifyPulseFixturesRechecksFreshSchemaWithoutRemoteOrJournalMutation(t *testing.T) {
	input := validPulseNativeInput(t)
	input.Path = filepath.Join(t.TempDir(), "source.tdsx")
	native := pulseTestTDSX(t)
	if err := os.WriteFile(input.Path, native, 0600); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(t.TempDir(), "journal.json")
	journal, err := harness.NewJournal(journalPath, "setup-run", "target", "tadx-live-pulse-test")
	if err != nil {
		t.Fatal(err)
	}
	projectName := "tadx-live-pulse-test-pulse-source-project"
	datasourceName := "tadx-live-pulse-test-pulse-source-datasource"
	if err := journal.AddIntent("project", projectName, "root"); err != nil {
		t.Fatal(err)
	}
	if err := journal.Track(harness.Object{Kind: "project", ID: "project-id", Name: projectName, ProjectID: "root"}); err != nil {
		t.Fatal(err)
	}
	if err := journal.AddIntent("datasource", datasourceName, "project-id"); err != nil {
		t.Fatal(err)
	}
	if err := journal.Track(harness.Object{Kind: "datasource", ID: "target-ds-id", Name: datasourceName, ProjectID: "project-id"}); err != nil {
		t.Fatal(err)
	}
	writes, schemaReads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/v1/vizql-data-service/") {
			writes++
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site/projects":
			_ = json.NewEncoder(w).Encode(map[string]any{"projects": map[string]any{"project": []any{map[string]any{"id": "project-id", "name": projectName, "parentProjectId": "root"}}}, "pagination": map[string]any{"totalAvailable": "1"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site/datasources/target-ds-id":
			_ = json.NewEncoder(w).Encode(map[string]any{"datasource": map[string]any{"id": "target-ds-id", "name": datasourceName, "project": map[string]any{"id": "project-id"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site/datasources/target-ds-id/content":
			w.Header().Set("Content-Disposition", `attachment; filename="source.tdsx"`)
			_, _ = w.Write(native)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/vizql-data-service/read-metadata":
			schemaReads++
			if r.Header.Get("X-Tableau-Site-Id") != "site" {
				t.Error("schema read omitted exact site header")
			}
			if schemaReads == 1 {
				_, _ = w.Write([]byte(`{"data":[{"fieldName":"[Order Date]","fieldCaption":"Order Date","dataType":"DATE","fieldRole":"DIMENSION","fieldType":"ORDINAL","columnClass":"COLUMN"},{"fieldName":"[Product Category]","fieldCaption":"Product Category","dataType":"STRING","fieldRole":"DIMENSION","fieldType":"NOMINAL","columnClass":"COLUMN"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"data":[{"fieldName":"[Sales]","fieldCaption":"Sales","dataType":"INTEGER","fieldRole":"MEASURE","fieldType":"CONTINUOUS","columnClass":"COLUMN"},{"fieldName":"[Order Date]","fieldCaption":"Order Date","dataType":"DATE","fieldRole":"DIMENSION","fieldType":"ORDINAL","columnClass":"COLUMN"},{"fieldName":"[Product Category]","fieldCaption":"Product Category","dataType":"STRING","fieldRole":"DIMENSION","fieldType":"NOMINAL","columnClass":"COLUMN"}]}`))
			}
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	profile := harness.Profile{Budgets: harness.Budgets{ObservationSeconds: 1}, Bindings: map[string]harness.Binding{
		pulseNativeBinding:     input,
		"pulse_fixture_source": {ID: "target-ds-id", Name: datasourceName, Params: map[string]string{"project_id": "project-id"}},
		"pulse_datasource":     {ID: "target-ds-id", Name: datasourceName, Params: map[string]string{"project_id": "project-id", "measure_field": "[Sales]", "date_field": "[Order Date]", "dimension_field": "[Product Category]", "category": "Electronics"}},
	}}
	c := &harness.Case{Root: "root", NamePrefix: journal.Prefix, Journal: journal, API: &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}, Profile: profile}
	before, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPulseFixtures(t.Context(), c, false); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || writes != 0 || schemaReads != 2 {
		t.Fatal("Pulse verification changed its journal or sent a remote write")
	}
}

func TestAwaitPulseFixturesPreservesDistinctProjectIDsForReadOnlyVerification(t *testing.T) {
	input := validPulseNativeInput(t)
	input.Path = filepath.Join(t.TempDir(), "source.tdsx")
	native := pulseTestTDSX(t)
	if err := os.WriteFile(input.Path, native, 0600); err != nil {
		t.Fatal(err)
	}
	input.Params["measure_name"], input.Params["date_name"], input.Params["dimension_name"] = "Sales", "Order Date", "Product Category"
	input.Expected["columns"] = []string{"[Sales]", "[Order Date]", "[Product Category]"}
	for _, field := range input.Expected["pulse_fields"].([]any) {
		item := field.(map[string]any)
		item["name"] = strings.Trim(item["name"].(string), "[]")
	}
	journalPath := filepath.Join(t.TempDir(), "journal.json")
	journal, err := harness.NewJournal(journalPath, "setup-run", "target", "tadx-live-pulse-test")
	if err != nil {
		t.Fatal(err)
	}
	projects := []struct{ role, projectID, datasourceID string }{
		{"source", "source-project-id", "source-ds-id"},
		{"destination", "destination-project-id", "destination-ds-id"},
	}
	for _, item := range projects {
		projectName := journal.Prefix + "-pulse-" + item.role + "-project"
		datasourceName := journal.Prefix + "-pulse-" + item.role + "-datasource"
		if err := journal.AddIntent("project", projectName, "root"); err != nil {
			t.Fatal(err)
		}
		if err := journal.Track(harness.Object{Kind: "project", ID: item.projectID, Name: projectName, ProjectID: "root"}); err != nil {
			t.Fatal(err)
		}
		if err := journal.AddIntent("datasource", datasourceName, item.projectID); err != nil {
			t.Fatal(err)
		}
		if err := journal.Track(harness.Object{Kind: "datasource", ID: item.datasourceID, Name: datasourceName, ProjectID: item.projectID}); err != nil {
			t.Fatal(err)
		}
	}
	writes, schemaReads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.URL.Path != "/api/v1/vizql-data-service/read-metadata" {
			writes++
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site/projects":
			items := make([]any, 0, len(projects))
			for _, item := range projects {
				items = append(items, map[string]any{"id": item.projectID, "name": journal.Prefix + "-pulse-" + item.role + "-project", "parentProjectId": "root"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"projects": map[string]any{"project": items}, "pagination": map[string]any{"totalAvailable": "2"}})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/3.29/sites/site/datasources/") && !strings.HasSuffix(r.URL.Path, "/content"):
			for _, item := range projects {
				if strings.HasSuffix(r.URL.Path, item.datasourceID) {
					_ = json.NewEncoder(w).Encode(map[string]any{"datasource": map[string]any{"id": item.datasourceID, "name": journal.Prefix + "-pulse-" + item.role + "-datasource", "project": map[string]any{"id": item.projectID}}})
					return
				}
			}
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/content"):
			w.Header().Set("Content-Disposition", `attachment; filename="source.tdsx"`)
			_, _ = w.Write(native)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/vizql-data-service/read-metadata":
			schemaReads++
			if r.Header.Get("X-Tableau-Site-Id") != "site" {
				t.Error("schema read omitted exact site header")
			}
			_, _ = w.Write([]byte(`{"data":[{"fieldName":"Sales","fieldCaption":"Sales","dataType":"INTEGER","fieldRole":"MEASURE","fieldType":"CONTINUOUS","columnClass":"COLUMN"},{"fieldName":"Order Date","fieldCaption":"Order Date","dataType":"DATE","fieldRole":"DIMENSION","fieldType":"ORDINAL","columnClass":"COLUMN"},{"fieldName":"Product Category","fieldCaption":"Product Category","dataType":"STRING","fieldRole":"DIMENSION","fieldType":"NOMINAL","columnClass":"COLUMN"}]}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	bindings := map[string]harness.Binding{pulseNativeBinding: input}
	for _, item := range projects {
		bindings["pulse_fixture_"+item.role] = harness.Binding{ID: item.datasourceID, Name: journal.Prefix + "-pulse-" + item.role + "-datasource", Params: map[string]string{"project_id": item.projectID}}
	}
	c := &harness.Case{Root: "root", NamePrefix: journal.Prefix, Journal: journal, API: &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}, Profile: harness.Profile{Budgets: harness.Budgets{ObservationSeconds: 1}, Bindings: bindings}}
	if err := AwaitPulseFixtures(t.Context(), c, true); err != nil {
		t.Fatal(err)
	}
	for _, item := range projects {
		ready := c.Profile.Bindings["pulse_"+map[string]string{"source": "datasource", "destination": "destination_datasource"}[item.role]]
		if ready.Params["project_id"] != item.projectID {
			t.Fatalf("%s ready binding lost its project identity", item.role)
		}
	}
	before, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPulseFixtures(t.Context(), c, true); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || writes != 0 || schemaReads != 4 {
		t.Fatal("Pulse verification changed its journal or sent a remote write")
	}
}

func validPulseNativeInput(t *testing.T) harness.Binding {
	t.Helper()
	path := filepath.Join(t.TempDir(), "portable-source.tdsx")
	if err := os.WriteFile(path, []byte("portable source bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	return harness.Binding{Path: path, Params: map[string]string{"measure_name": "[Sales]", "date_name": "[Order Date]", "dimension_name": "[Product Category]", "category": "Electronics"}, Expected: map[string]any{
		"columns": []string{"[Sales]", "[Order Date]", "[Product Category]"}, "extract_count": 1,
		"pulse_fields": []any{
			map[string]any{"name": "[Sales]", "caption": "Sales", "type": "integer", "role": "measure"},
			map[string]any{"name": "[Order Date]", "caption": "Order Date", "type": "date", "role": "dimension"},
			map[string]any{"name": "[Product Category]", "caption": "Product Category", "type": "text", "role": "dimension"},
		},
	}}
}

func pulseTestTDSX(t *testing.T) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	document, err := writer.Create("source.tds")
	if err != nil {
		t.Fatal(err)
	}
	_, err = document.Write([]byte(`<datasource><column name="[Sales]" datatype="integer"/><column name="[Order Date]" datatype="date"/><column name="[Product Category]" datatype="string"/></datasource>`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Create("Data/Extract.hyper"); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}
