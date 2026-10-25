package suite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func TestSharedCacheOracleRejectsWrongTypedRelationship(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/3.29/sites/site/workbooks" {
			t.Errorf("unexpected oracle request %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pagination":{"totalAvailable":"1"},"workbooks":{"workbook":[{"id":"fixture","name":"Fixture","project":{"id":"project"},"owner":{"id":"real-owner"}}]}}`))
	}))
	defer server.Close()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "catalog"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "catalog", "target-owned.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE workbooks (generation_key INTEGER,id TEXT,name TEXT,project_id TEXT,owner_id TEXT); INSERT INTO workbooks VALUES(1,'fixture','Fixture','project','real-owner')`); err != nil {
		t.Fatal(err)
	}
	c := &harness.Case{ConfigPath: filepath.Join(root, "config.yaml"), API: &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}}
	readonly, err := sharedOpenCache(c)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	if err = sharedCacheRows(t.Context(), c, readonly, sharedGeneration{Key: 1}, "workbook"); err != nil {
		t.Fatalf("valid persisted relationships rejected: %v", err)
	}
	if _, err = db.Exec(`UPDATE workbooks SET owner_id='wrong-owner'`); err != nil {
		t.Fatal(err)
	}
	if err = sharedCacheRows(t.Context(), c, readonly, sharedGeneration{Key: 1}, "workbook"); err == nil {
		t.Fatal("wrong owner passes cache qualification")
	}
	if _, err = readonly.Exec(`DELETE FROM workbooks`); err == nil {
		t.Fatal("independent cache observer unexpectedly has write access")
	}
}

func TestSharedAuthChecksActorAgainstIndependentIdentity(t *testing.T) {
	binary := contentCandidate(t)
	root := t.TempDir()
	c := &harness.Case{Binary: binary, RunDir: root, ConfigPath: filepath.Join(root, "config.yaml"), Environment: "fixture", ActorUserID: "actor", API: &harness.Client{SiteID: "site"}, Profile: harness.Profile{Budgets: harness.Budgets{CommandSeconds: 20}}, ExtraEnv: []string{`LIVE_CANDIDATE_JSON={"status":"authenticated","site_luid":"site","user_luid":"actor"}`}}
	if err := sharedAuthCheck(t.Context(), c); err != nil {
		t.Fatalf("valid independent actor rejected: %v", err)
	}
	c.ActorUserID = "different-actor"
	if err := sharedAuthCheck(t.Context(), c); err == nil {
		t.Fatal("authenticated output for another user passes")
	}
}

func TestSharedTOONKnownFieldsDecodeQuotedNames(t *testing.T) {
	facts, err := sharedTOONFacts("status: found\nworkbook:\n  luid: workbook-id\n  name: \"Sales: North, West\"\nhelp[1]: command\n")
	if err != nil {
		t.Fatal(err)
	}
	if facts["workbook.luid"] != "workbook-id" || facts["workbook.name"] != "Sales: North, West" {
		t.Fatalf("independent scalar decode loses identity facts: %v", facts)
	}
	if _, err = sharedTOONFacts("status: found\nstatus: failed\n"); err == nil {
		t.Fatal("duplicate scalar facts accepted")
	}
}

func TestSharedContentCacheScopesCoverAggregateContentSearch(t *testing.T) {
	want := []string{"projects", "workbooks", "datasources", "flows", "views"}
	if got := sharedContentCacheScopes(); !slices.Equal(got, want) {
		t.Fatalf("content cache scopes = %v, want %v", got, want)
	}
}

func TestSharedContentCacheBlocksExplicitlyUnsupportedFlowInventory(t *testing.T) {
	c := &harness.Case{Profile: harness.Profile{Features: map[string]string{"flows": "unsupported"}}}
	err := sharedCache("content")(t.Context(), c)
	if harness.ErrorStatus(err) != harness.UnsupportedStatus {
		t.Fatalf("explicitly unsupported flow inventory status = %q, want %q: %v", harness.ErrorStatus(err), harness.UnsupportedStatus, err)
	}
}

func TestSharedSearchFactsAllowsBroadBoundedRelevanceResults(t *testing.T) {
	expected := []harness.Object{
		{ID: "project-discovery", Name: "Fixture project", Kind: "project"},
		{ID: "workbook-discovery", Name: "Fixture workbook", Kind: "workbook"},
		{ID: "datasource-discovery", Name: "Fixture datasource", Kind: "datasource"},
	}
	rows := []any{
		map[string]any{"luid": "project-discovery", "name": "Fixture project", "type": "project"},
		map[string]any{"luid": "workbook-discovery", "name": "Fixture workbook", "type": "workbook"},
		map[string]any{"luid": "datasource-discovery", "name": "Fixture datasource", "type": "datasource"},
		map[string]any{"luid": "project-contrast", "name": "Contrast project", "type": "project"},
	}
	for i := 0; i < 96; i++ {
		rows = append(rows, map[string]any{"luid": fmt.Sprintf("unrelated-%d", i), "name": fmt.Sprintf("Related item %d", i), "type": "workbook"})
	}
	out := map[string]any{"source": "live", "items": rows, "page": map[string]any{"limit": 100, "returned": 100, "more_available": true}}
	if err := sharedSearchFacts(out, expected); err != nil {
		t.Fatalf("complete expected set on broad bounded page rejected: %v", err)
	}
}

func TestSharedSearchFactsParsesWireNumbersWithUseNumber(t *testing.T) {
	const wire = `{"source":"live","items":[{"luid":"workbook-wire","name":"Fixture workbook","type":"workbook"}],"page":{"limit":100,"returned":1,"more_available":true}}`
	decoder := json.NewDecoder(strings.NewReader(wire))
	decoder.UseNumber()
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		t.Fatal(err)
	}
	expected := []harness.Object{{ID: "workbook-wire", Name: "Fixture workbook", Kind: "workbook"}}
	if err := sharedSearchFacts(out, expected); err != nil {
		t.Fatalf("UseNumber wire response rejected: %v", err)
	}
	for _, value := range []any{
		json.Number("-1"), json.Number("1.5"), json.Number("invalid"), json.Number("999999999999999999999999999999999999"),
		"1", float64(-1), float64(1.5),
	} {
		if _, ok := searchPageInteger(value); ok {
			t.Fatalf("invalid page integer accepted: %T", value)
		}
	}
	if value, ok := searchPageInteger(json.Number("0")); !ok || value != 0 {
		t.Fatalf("nonnegative returned count rejected: value=%d ok=%t", value, ok)
	}
}

func TestSharedSearchFactsRequiresEachExpectedIdentityExactly(t *testing.T) {
	expected := []harness.Object{{ID: "workbook-1", Name: "Fixture workbook", Kind: "workbook"}}
	base := func() map[string]any {
		return map[string]any{"source": "live", "items": []any{map[string]any{"luid": "workbook-1", "name": "Fixture workbook", "type": "workbook"}, map[string]any{"luid": "other-1", "name": "Other", "type": "workbook"}}, "page": map[string]any{"limit": 100, "returned": 2, "more_available": true}}
	}
	for name, mutate := range map[string]func(map[string]any){
		"missing": func(out map[string]any) {
			out["items"] = []any{map[string]any{"luid": "other-1", "name": "Other", "type": "workbook"}}
			contentMap(out["page"])["returned"] = 1
		},
		"wrong type": func(out map[string]any) { contentMap(contentMap(out["items"].([]any)[0]))["type"] = "datasource" },
		"wrong name": func(out map[string]any) { contentMap(contentMap(out["items"].([]any)[0]))["name"] = "Different" },
		"duplicate": func(out map[string]any) {
			out["items"] = append(out["items"].([]any), map[string]any{"luid": "workbook-1", "name": "Fixture workbook", "type": "workbook"})
			contentMap(out["page"])["returned"] = 3
		},
		"bad page count":    func(out map[string]any) { contentMap(out["page"])["returned"] = 99 },
		"missing more flag": func(out map[string]any) { delete(contentMap(out["page"]), "more_available") },
		"missing source":    func(out map[string]any) { delete(out, "source") },
		"unknown source":    func(out map[string]any) { out["source"] = "fallback" },
	} {
		t.Run(name, func(t *testing.T) {
			out := base()
			mutate(out)
			if err := sharedSearchFacts(out, expected); err == nil {
				t.Fatal("invalid or incomplete search page accepted")
			}
		})
	}
}

func TestSharedSearchFixtureNamesIsolateOpaqueTokenFromContrast(t *testing.T) {
	const prefix = "tadx-live-search-case"
	const token = "0123456789abcdefabcd"
	contrast := harness.Object{Name: sharedContentFixtureName(prefix, "", "contrast"), Description: "Disposable fixture", ProjectID: "root-project", Fields: map[string]any{"name": sharedContentFixtureName(prefix, "", "contrast"), "description": "Disposable fixture", "parentProjectId": "root-project", "parentProjectPath": "root-project"}}
	expected := []harness.Object{
		{Kind: "project", Name: sharedContentFixtureName(prefix, token, "")},
		{Kind: "workbook", Name: sharedContentFixtureName(prefix, token, "workbook")},
		{Kind: "datasource", Name: sharedContentFixtureName(prefix, token, "datasource")},
		{Kind: "flow", Name: sharedContentFixtureName(prefix, token, "flow")},
	}
	if err := sharedSearchFixtureNamesIsolated(prefix, token, contrast, expected); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range expected {
		if !strings.Contains(fixture.Name, token) {
			t.Fatalf("matching fixture name omits isolated token: %q", fixture.Name)
		}
	}
	contrast.Fields["description"] = "contains " + token
	if err := sharedSearchFixtureNamesIsolated(prefix, token, contrast, expected); err == nil {
		t.Fatal("contrast description containing isolated token accepted")
	}
	contrast.Fields["description"] = "Disposable fixture"
	contrast.Fields["parentProjectPath"] = "root/" + token
	if err := sharedSearchFixtureNamesIsolated(prefix, token, contrast, expected); err == nil {
		t.Fatal("contrast parent path containing isolated token accepted")
	}
}

func TestSharedAdminSearchNeedleUsesExactOwnedUserToken(t *testing.T) {
	caseData, user := sharedAdminSearchCase(t)
	token, err := sharedAdminSearchToken(caseData, user)
	if err != nil {
		t.Fatalf("exact owned user token rejected: %v", err)
	}
	entry := caseData.Journal.Entries[0]
	if token == "" || token != entry.UserToken || !strings.Contains(user.Name, token) || strings.Contains(caseData.NamePrefix, token) {
		t.Fatal("admin search needle does not match the exact controlled-user token")
	}
	groupName := caseData.NamePrefix + "-discovery-" + token
	if !strings.HasPrefix(groupName, caseData.NamePrefix+"-") || !strings.Contains(groupName, token) {
		t.Fatal("admin search group name does not share the controlled-user token")
	}
}

func TestSharedAdminSearchNeedleRejectsUnrelatedOrChangedReceipt(t *testing.T) {
	caseData, user := sharedAdminSearchCase(t)
	unrelated := user
	unrelated.ID = "different-user"
	if _, err := sharedAdminSearchToken(caseData, unrelated); err == nil {
		t.Fatal("unrelated user identity accepted for the admin search needle")
	}
	original := caseData.Journal.Entries[0].UserToken
	caseData.Journal.Entries[0].UserToken = original + "f"
	if _, err := sharedAdminSearchToken(caseData, user); err == nil {
		t.Fatal("changed recorded user token accepted for the admin search needle")
	}
}

func sharedAdminSearchCase(t *testing.T) (*harness.Case, harness.Object) {
	t.Helper()
	const prefix = "tadx-live-search-case"
	journal, err := harness.NewJournal(filepath.Join(t.TempDir(), "journal.json"), "0123456789abcdefabcd", "target", prefix)
	if err != nil {
		t.Fatalf("create controlled-user journal: %v", err)
	}
	caseData := &harness.Case{
		NamePrefix: prefix,
		Journal:    journal,
		Profile: harness.Profile{Bindings: map[string]harness.Binding{
			"controlled_user": {Name: "ahillspace+{prefix}@gmail.com"},
		}},
	}
	login, err := caseData.IntentControlledUser("discovery")
	if err != nil {
		t.Fatalf("record controlled-user intent: %v", err)
	}
	user := harness.Object{Kind: "user", ID: "user-identity", Name: login}
	if err := caseData.ConfirmControlledUserReceipt("discovery", login, user); err != nil {
		t.Fatalf("confirm controlled-user receipt: %v", err)
	}
	return caseData, user
}

func TestSharedBatchOutcomesRequireCanonicalOrderedIdentities(t *testing.T) {
	out := map[string]any{"items": []any{
		map[string]any{"selector": "id=workbook-1", "status": "succeeded", "result": map[string]any{"result": map[string]any{"workbook_luid": "workbook-1"}}},
		map[string]any{"selector": "id=missing-1", "status": "failed"},
	}}
	if err := sharedValidateBatchOutcomes(out, "workbook-1", "missing-1"); err != nil {
		t.Fatalf("canonical ordered batch outcomes rejected: %v", err)
	}

	for name, mutate := range map[string]func([]any){
		"raw-selector": func(items []any) { contentMap(items[0])["selector"] = "workbook-1" },
		"reordered":    func(items []any) { items[0], items[1] = items[1], items[0] },
		"changed-id":   func(items []any) { contentMap(items[1])["selector"] = "id=other-1" },
		"changed-status": func(items []any) {
			contentMap(items[0])["status"] = "failed"
		},
		"changed-success-identity": func(items []any) {
			contentMap(contentMap(contentMap(items[0])["result"])["result"])["workbook_luid"] = "other-1"
		},
	} {
		t.Run(name, func(t *testing.T) {
			items := []any{
				map[string]any{"selector": "id=workbook-1", "status": "succeeded", "result": map[string]any{"result": map[string]any{"workbook_luid": "workbook-1"}}},
				map[string]any{"selector": "id=missing-1", "status": "failed"},
			}
			mutate(items)
			if err := sharedValidateBatchOutcomes(map[string]any{"items": items}, "workbook-1", "missing-1"); err == nil {
				t.Fatal("mutated batch outcome passes identity/order contract")
			}
		})
	}
}

func TestSharedOutputUsesSetupOwnedRetainedWorkbookPair(t *testing.T) {
	const id = "live.contract.output-real-result"
	if got := SharedBindingRequirements()[id]; len(got) != 0 {
		t.Fatalf("output authored bindings = %v, want none because setup supplies retained fixtures", got)
	}
	if caps := SharedFixtureCapabilities(id); len(caps) != 0 {
		t.Fatalf("output fixture capabilities = %v, want none for retained read-only fixtures", caps)
	}
}

func TestSharedOutputDetailsCompareExpandedFieldsWithIndependentTruth(t *testing.T) {
	oracle := harness.Object{ID: "workbook-1", OwnerID: "owner-1", Fields: map[string]any{"createdAt": "2026-10-24T12:30:00.000Z"}}
	full := map[string]any{"owner_luid": "owner-1", "created_at": "2026-10-24T12:30:00Z"}
	compact := map[string]any{"luid": "workbook-1", "name": "Retained"}
	if err := sharedOutputDetails(full, compact, oracle); err != nil {
		t.Fatalf("full output with empty optional description rejected: %v", err)
	}
	for name, mutate := range map[string]func(map[string]any, map[string]any){
		"wrong owner":           func(expanded, _ map[string]any) { expanded["owner_luid"] = "other-owner" },
		"missing creation time": func(expanded, _ map[string]any) { delete(expanded, "created_at") },
		"wrong creation time":   func(expanded, _ map[string]any) { expanded["created_at"] = "2026-10-24T12:31:00Z" },
		"compact owner":         func(_, compact map[string]any) { compact["owner_luid"] = "owner-1" },
		"compact creation time": func(_, compact map[string]any) { compact["created_at"] = "2026-10-24T12:30:00Z" },
	} {
		t.Run(name, func(t *testing.T) {
			expanded := map[string]any{"owner_luid": "owner-1", "created_at": "2026-10-24T12:30:00Z"}
			compactOutput := map[string]any{"luid": "workbook-1", "name": "Retained"}
			mutate(expanded, compactOutput)
			if err := sharedOutputDetails(expanded, compactOutput, oracle); err == nil {
				t.Fatal("invalid full/compact detail contract accepted")
			}
		})
	}
}

func TestCatalogLabelUsesCanonicalDatasourceType(t *testing.T) {
	got, err := catalogLabelType("DATASOURCE")
	if err != nil || got != "datasource" {
		t.Fatalf("canonical type = %q, err=%v", got, err)
	}
	for _, raw := range []string{"database", "Datasource", "VIRTUALCONNECTION"} {
		if got, err := catalogLabelType(raw); err == nil || got != "" {
			t.Fatalf("unexpected REST type %q normalized to %q, err=%v", raw, got, err)
		}
	}
}

func TestSharedRequirementsCoverRegisteredScenarios(t *testing.T) {
	cases, requirements := SharedCases(), SharedBindingRequirements()
	if len(cases) != 15 || len(requirements) != 15 {
		t.Fatalf("cases/requirements=%d/%d", len(cases), len(requirements))
	}
	for id := range cases {
		if _, ok := requirements[id]; !ok {
			t.Errorf("missing setup declaration for %s", id)
		}
	}
	if err := ValidateSharedBindings("live.auth.login-native-store", harness.Profile{Bindings: map[string]harness.Binding{"native_login": {Params: map[string]string{"operator_assisted": "true"}}}}); err == nil {
		t.Fatal("native login without explicit persistence consent accepted")
	}
}
