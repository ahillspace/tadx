package suite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func TestWorkbookInspectWaitsForExactMetadataIdentity(t *testing.T) {
	for _, wrong := range []bool{false, true} {
		t.Run(map[bool]string{false: "eventual indexing", true: "wrong identity"}[wrong], func(t *testing.T) {
			var reads atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Query     string         `json:"query"`
					Variables map[string]any `json:"variables"`
				}
				if r.Method != "POST" || r.URL.Path != "/api/metadata/graphql" || json.NewDecoder(r.Body).Decode(&request) != nil || request.Variables["luid"] != "exact-workbook" || !strings.Contains(request.Query, "workbooksConnection") {
					t.Error("readiness does not use an exact independent Metadata query")
					w.WriteHeader(400)
					return
				}
				count := 1
				nodes := []any{map[string]any{"id": "metadata-workbook", "luid": "exact-workbook", "name": "Expected workbook"}}
				if reads.Add(1) == 1 && !wrong {
					count, nodes = 0, []any{}
				}
				if wrong {
					nodes[0].(map[string]any)["luid"] = "same-name-decoy"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"workbooks": map[string]any{"totalCount": count, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": nodes}}})
			}))
			defer server.Close()
			c := &harness.Case{API: &harness.Client{Server: server.URL, HTTP: server.Client()}, Profile: harness.Profile{Budgets: harness.Budgets{ObservationSeconds: 1}}}
			ctx, cancel := context.WithTimeout(t.Context(), 600*time.Millisecond)
			defer cancel()
			err := contentAwaitWorkbookMetadata(ctx, c, harness.Object{ID: "exact-workbook", Name: "Expected workbook"})
			if wrong && err == nil {
				t.Fatal("same-name Metadata decoy accepted")
			}
			if !wrong && (err != nil || reads.Load() < 2) {
				t.Fatalf("indexing readiness err=%v reads=%d", err, reads.Load())
			}
		})
	}
}

func TestNativeSeedPreservesSparseAcceptedIDBeforeVisibility(t *testing.T) {
	root := t.TempDir()
	journal, err := harness.NewJournal(filepath.Join(root, "journal.json"), "run", "target", "tadx-live-case")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "fixture.tds")
	if err = os.WriteFile(path, []byte(`<datasource><column name="[ID]" datatype="integer"/></datasource>`), 0600); err != nil {
		t.Fatal(err)
	}
	var submissions, reads atomic.Int64
	name := "tadx-live-case-source"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/datasources"):
			submissions.Add(1)
			_, _ = w.Write([]byte(`{"job":{"id":"accepted-job"}}`))
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/jobs/accepted-job"):
			_, _ = w.Write([]byte(`{"job":{"id":"accepted-job","progress":"100","finishCode":"0","datasource":{"id":"accepted-source"}}}`))
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/datasources/accepted-source"):
			if journal.Entries[0].Object == nil || journal.Entries[0].Object.ID != "accepted-source" {
				t.Error("sparse accepted identity is not saved before readback")
			}
			if reads.Add(1) == 1 {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"datasource": map[string]any{"id": "accepted-source", "name": name, "project": map[string]any{"id": "project"}}})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/datasources"):
			_ = json.NewEncoder(w).Encode(map[string]any{"pagination": map[string]any{"totalAvailable": 1}, "datasources": map[string]any{"datasource": []any{map[string]any{"id": "accepted-source", "name": name, "project": map[string]any{"id": "project"}}}}})
		default:
			t.Errorf("unexpected fixture request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	c := &harness.Case{Journal: journal, NamePrefix: "tadx-live-case", API: &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}, Profile: harness.Profile{Budgets: harness.Budgets{ObservationSeconds: 1}, Bindings: map[string]harness.Binding{"datasource-native": {Path: path, Expected: map[string]any{"columns": []any{"[ID]"}}}}}}
	object, err := contentPublishSeed(t.Context(), c, "datasource", harness.Object{ID: "project"}, name, "")
	if err != nil {
		t.Fatal(err)
	}
	if object.ID != "accepted-source" || object.Name != name || object.ProjectID != "project" {
		t.Fatalf("sparse receipt did not hydrate exact identity: %#v", object)
	}
	if submissions.Load() != 1 || reads.Load() < 2 {
		t.Fatalf("seed submission/read counts=%d/%d", submissions.Load(), reads.Load())
	}
}

func TestDetachedInspectionUsesRenderedSnapshotTracking(t *testing.T) {
	var output map[string]any
	if err := json.Unmarshal([]byte(`{"operation_id":"run-accepted","operation":"datasource.publish","status":"succeeded","result":{"datasource_luid":"published","tableau_job_id":"remote-job","receipt_path":"saved-receipt.json"},"tracking":{"phase":"completed","worker_alive":false}}`), &output); err != nil {
		t.Fatal(err)
	}
	phase, alive, err := contentOperationTracking(output, "run-accepted")
	if err != nil || phase != "completed" || alive {
		t.Fatalf("actual rendered contract rejected: phase=%q alive=%v err=%v", phase, alive, err)
	}
	if _, _, err = contentOperationTracking(output, "another-operation"); err == nil {
		t.Fatal("wrong operation accepted")
	}
	delete(contentMap(output["tracking"]), "worker_alive")
	if _, _, err = contentOperationTracking(output, "run-accepted"); err == nil {
		t.Fatal("missing worker liveness inferred as stopped")
	}
}

func TestProjectInventoryExhaustsPagesAndRejectsSameNameDecoy(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		t.Run(kind, func(t *testing.T) {
			var reads atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/"+kind+"s") {
					t.Errorf("unexpected independent request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
					return
				}
				if r.URL.Query().Get("filter") != "" {
					t.Error("oracle sends unsupported projectId filtering upstream")
					w.WriteHeader(400)
					return
				}
				reads.Add(1)
				var records []any
				switch r.URL.Query().Get("pageNumber") {
				case "1":
					records = []any{map[string]any{"id": "selected-a", "name": "Same name", "project": map[string]any{"id": "selected-project"}}, map[string]any{"id": "decoy", "name": "Same name", "project": map[string]any{"id": "another-project"}}}
				case "2":
					records = []any{map[string]any{"id": "selected-b", "name": "Later page", "project": map[string]any{"id": "selected-project"}}}
				default:
					t.Errorf("unexpected inventory page %q", r.URL.Query().Get("pageNumber"))
					w.WriteHeader(500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"pagination": map[string]any{"totalAvailable": 3}, kind + "s": map[string]any{kind: records}})
			}))
			defer server.Close()
			c := &harness.Case{API: &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}}
			objects, err := contentProjectInventory(t.Context(), c, kind, "selected-project")
			if err != nil {
				t.Fatal(err)
			}
			ids := contentObjectIDs(objects)
			if len(ids) != 2 || ids[0] != "selected-a" || ids[1] != "selected-b" {
				t.Fatalf("scoped identities=%v, want two direct members across pages", ids)
			}
			if reads.Load() != 2 {
				t.Fatalf("independent complete inventory reads=%d, want2", reads.Load())
			}
		})
	}
}

func TestCreatedProjectObservesDelayedVisibilityWithoutResubmission(t *testing.T) {
	for _, wrong := range []bool{false, true} {
		t.Run(map[bool]string{false: "delayed-visible", true: "stable-wrong-metadata"}[wrong], func(t *testing.T) {
			root := t.TempDir()
			journal, err := harness.NewJournal(filepath.Join(root, "journal.json"), "run", "target", "tadx-live-case")
			if err != nil {
				t.Fatal(err)
			}
			name := "tadx-live-case-child"
			if err = journal.AddIntent("project", name, "parent"); err != nil {
				t.Fatal(err)
			}
			var reads atomic.Int64
			var writes atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes.Add(1)
					w.WriteHeader(500)
					return
				}
				if !strings.HasSuffix(r.URL.Path, "/projects") {
					t.Errorf("unexpected observation %s", r.URL.Path)
				}
				if journal.Entries[0].Object == nil || journal.Entries[0].Object.ID != "created-child" {
					t.Error("accepted receipt identity was not saved before independent observation")
				}
				count := reads.Add(1)
				objects := []any{map[string]any{"id": "parent", "name": "tadx-live-case-parent", "parentProjectId": "root"}}
				if count > 1 || wrong {
					description := "Live project fixture"
					if wrong {
						description = "Contrasting unexpected payload"
					}
					objects = append(objects, map[string]any{"id": "created-child", "name": name, "parentProjectId": "parent", "description": description, "contentPermissions": "ManagedByOwner"})
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"pagination": map[string]any{"totalAvailable": len(objects)}, "projects": map[string]any{"project": objects}})
			}))
			defer server.Close()
			c := &harness.Case{Journal: journal, API: &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}, Profile: harness.Profile{Budgets: harness.Budgets{ObservationSeconds: 1}}}
			receipt := map[string]any{"result": map[string]any{"project": map[string]any{"luid": "created-child", "name": name, "parent_luid": "parent"}}}
			parent := harness.Object{Kind: "project", ID: "parent", Name: "tadx-live-case-parent", ProjectID: "root"}
			ctx := t.Context()
			if wrong {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 40*time.Millisecond)
				defer cancel()
			}
			err = contentObserveCreatedProject(ctx, c, parent, name, receipt)
			if (err == nil) == wrong {
				t.Fatalf("observation error=%v, wrong metadata=%v", err, wrong)
			}
			if !wrong && reads.Load() < 2 {
				t.Fatal("delayed fixture never receives a second independent observation")
			}
			if writes.Load() != 0 {
				t.Fatal("observation retries a mutation")
			}
		})
	}
}
