package suite

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func TestAdminUserInspectRejectsCandidateIdentityAndRoleDrift(t *testing.T) {
	binary := contentCandidate(t)
	for _, mutation := range []string{"correct", "wrong-role", "wrong-login", "wrong-id"} {
		t.Run(mutation, func(t *testing.T) {
			reads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/users/user-a") {
					t.Errorf("unexpected REST request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
					return
				}
				reads++
				_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": "user-a", "name": "fixture@example.test", "siteRole": "Viewer"}})
			}))
			defer server.Close()
			row := map[string]any{"luid": "user-a", "name": "fixture@example.test", "site_role": "Viewer"}
			switch mutation {
			case "wrong-role":
				row["site_role"] = "Creator"
			case "wrong-login":
				row["name"] = "decoy@example.test"
			case "wrong-id":
				row["luid"] = "decoy"
			}
			data, _ := json.Marshal(map[string]any{"user": row})
			c := testAdminCase(t, server, binary, string(data))
			c.Profile.Bindings = map[string]harness.Binding{"user_a": {ID: "user-a"}}
			err := adminUser("inspect")(t.Context(), c)
			if (err == nil) != (mutation == "correct") {
				t.Fatalf("mutation %s: %v", mutation, err)
			}
			if reads != 1 {
				t.Fatalf("independent REST reads=%d", reads)
			}
		})
	}
}

func TestAdminUserCreateUsesExactApprovedAliasAndOwnedJournalKey(t *testing.T) {
	binary := contentCandidate(t)
	reads := 0
	createdID := "created-controlled-user"
	var createdLogin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/users") {
			t.Errorf("unexpected REST request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		reads++
		users := []any{}
		if reads > 1 {
			users = append(users, map[string]any{"id": createdID, "name": createdLogin, "siteRole": "Unlicensed", "authSetting": "ServerDefault"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"users": map[string]any{"user": users}, "pagination": map[string]any{"totalAvailable": len(users)}})
	}))
	defer server.Close()
	login := ""
	output, _ := json.Marshal(map[string]any{"user": map[string]any{"luid": createdID, "name": "placeholder", "site_role": "Unlicensed"}})
	c := testAdminCase(t, server, binary, string(output))
	c.Profile.Bindings = map[string]harness.Binding{"controlled_user": {Name: "controlled+{prefix}@example.test", Params: map[string]string{"auth_setting": "ServerDefault"}}}
	var err error
	login, err = c.ControlledUserLogin("admin-user-create")
	if err != nil {
		t.Fatal(err)
	}
	createdLogin = login
	output, _ = json.Marshal(map[string]any{"user": map[string]any{"luid": createdID, "name": login, "site_role": "Unlicensed"}})
	c.ExtraEnv = []string{"LIVE_CANDIDATE_JSON=" + string(output)}
	if err := adminUser("create")(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	if reads != 2 || len(c.Journal.Entries) != 1 || c.Journal.Entries[0].UserLogin != login || c.Journal.Entries[0].Name != c.NamePrefix+"-user-admin-user-create" || c.Journal.Entries[0].Object.ID != createdID {
		t.Fatal("admin user create did not use the exact generated alias and confirmed owned identity")
	}
}

func testAdminCase(t *testing.T, server *httptest.Server, binary, output string) *harness.Case {
	t.Helper()
	root := t.TempDir()
	journal, err := harness.NewJournal(filepath.Join(root, "journal.json"), "0123456789abcdefabcd", "target", "tadx-live-case")
	if err != nil {
		t.Fatal(err)
	}
	return &harness.Case{Root: "project", RunDir: root, WorkspacePath: root, ConfigPath: filepath.Join(root, "config.json"), Binary: binary, Environment: "fixture", NamePrefix: "tadx-live-case", Journal: journal, API: &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}, ExtraEnv: []string{"LIVE_CANDIDATE_JSON=" + output}, Profile: harness.Profile{Budgets: harness.Budgets{CommandSeconds: 20}, FixturePAT: harness.Credential{CredentialRef: "test-independent-fixture"}}}
}

func TestPermissionOracleMatchesPrincipalAndRuleTogether(t *testing.T) {
	doc := map[string]any{"permissions": map[string]any{"granteeCapabilities": []any{map[string]any{"group": map[string]any{"id": "target"}, "capabilities": map[string]any{"capability": []any{map[string]any{"name": "Read", "mode": "Deny"}}}}, map[string]any{"group": map[string]any{"id": "sentinel"}, "capabilities": map[string]any{"capability": []any{map[string]any{"name": "Read", "mode": "Allow"}}}}}}}
	if permissionPresent(doc, "target", "Read", "Allow") {
		t.Fatal("rule from contrasting principal satisfied target assertion")
	}
	if !permissionPresent(doc, "target", "Read", "Deny") {
		t.Fatal("exact independently read rule was not recognized")
	}
}

func TestAssignedBindingRequirementsCoverEveryScenario(t *testing.T) {
	requirements := BindingRequirements()
	count := 0
	for _, cases := range []map[string]harness.Scenario{AdminCases(), CatalogCases(), PulseCases()} {
		for id := range cases {
			count++
			if _, ok := requirements[id]; !ok {
				t.Fatalf("scenario %s has no setup dependency declaration", id)
			}
		}
	}
	if len(requirements) != count {
		t.Fatalf("dependency declarations include stale scenario IDs: got %d, want %d", len(requirements), count)
	}
}

func TestAssignedBindingValidationBlocksEmptyAuditTruth(t *testing.T) {
	profile := harness.Profile{Bindings: map[string]harness.Binding{"audit_table": {ID: "table-a", Params: map[string]string{"oracle_query": "query { fixture }"}, Expected: map[string]any{"oracle": map[string]any{"fixture": "known"}, "output": map[string]any{"findings": []any{}, "scanned": 0, "complete": true, "summary": map[string]any{"present": 0, "missing": 0, "unknown": 0}}}}}}
	if err := ValidateAssignedBindings("live.catalog.audit-table", profile); harness.ErrorStatus(err) != harness.BlockedStatus {
		t.Fatalf("empty audit truth status %v", err)
	}
}

func TestAdminGroupCreateVerifiesMutationReceiptAgainstIndependentInventory(t *testing.T) {
	binary := contentCandidate(t)
	for _, test := range []struct {
		name     string
		result   map[string]any
		wantPass bool
	}{
		{"created-group-receipt", map[string]any{"group_luid": "group-created", "status": "created"}, true},
		{"wrong-group-receipt", map[string]any{"group_luid": "group-decoy", "status": "created"}, false},
		{"missing-group-receipt", map[string]any{"status": "created"}, false},
		{"unconfirmed-receipt", map[string]any{"group_luid": "group-created", "status": "unknown"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			reads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/groups") {
					t.Errorf("unexpected independent request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				reads++
				groups := []any{}
				if reads > 1 {
					groups = append(groups, map[string]any{"id": "group-created", "name": "tadx-live-casegroup"})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"groups": map[string]any{"group": groups}, "pagination": map[string]any{"totalAvailable": len(groups), "pageNumber": 1, "pageSize": 100}})
			}))
			defer server.Close()
			output, _ := json.Marshal(map[string]any{"result": test.result, "unrelated": map[string]any{"luid": "group-created"}})
			c := testAdminCase(t, server, binary, string(output))
			err := adminGroup("create")(t.Context(), c)
			if (err == nil) != test.wantPass {
				t.Fatalf("%s: %v", test.name, err)
			}
			if reads != 2 {
				t.Fatalf("creation requires independent absent/present inventories; got %d", reads)
			}
		})
	}
}
