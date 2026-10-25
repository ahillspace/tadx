package harness

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func testJournal(t *testing.T) *Journal {
	t.Helper()
	journal, err := NewJournal(filepath.Join(t.TempDir(), "journal.json"), "run", "target-fingerprint", "tadx-live-safety")
	if err != nil {
		t.Fatal(err)
	}
	return journal
}
func own(t *testing.T, j *Journal, object Object) {
	t.Helper()
	if err := j.AddIntent(object.Kind, object.Name, object.ProjectID); err != nil {
		t.Fatal(err)
	}
	if err := j.Track(object); err != nil {
		t.Fatal(err)
	}
}
func observationClient(server *httptest.Server) *Client {
	return &Client{Server: server.URL, Version: "3.29", SiteID: "site-one", HTTP: server.Client()}
}

func TestCleanupJournalCannotLoadForAnotherTarget(t *testing.T) {
	j := testJournal(t)
	if _, err := LoadJournal(j.path, "different-target"); err == nil {
		t.Fatal("cross-target cleanup journal accepted")
	}
}

func TestJournalRejectsAdoptionWithoutPriorOwnedIntent(t *testing.T) {
	j := testJournal(t)
	object := Object{Kind: "group", ID: "group-one", Name: "tadx-live-safety-owned"}
	if err := j.Track(object); err == nil {
		t.Fatal("adopted remote identity without prior ownership intent")
	}
	if err := j.AddIntent("group", "unrelated-existing-group", ""); err == nil {
		t.Fatal("adopted resource outside generated namespace")
	}
	own(t, j, object)
	if len(j.Entries) != 1 || j.Entries[0].Object.ID != "group-one" {
		t.Fatal("valid prior ownership intent could not confirm resource")
	}
}

func TestSparseReceiptRequiresExactPriorRequestAndPreservesReturnedFields(t *testing.T) {
	j := testJournal(t)
	c := &Case{Journal: j}
	receipt := Object{Kind: "datasource", ID: "announced-id"}
	name := "tadx-live-safety-source"
	if err := c.ConfirmReceipt("datasource", name, "project-one", receipt); err == nil {
		t.Fatal("sparse receipt adopted without prior ownership request")
	}
	if err := j.AddIntent("datasource", name, "project-one"); err != nil {
		t.Fatal(err)
	}
	if err := j.Track(receipt); err == nil {
		t.Fatal("generic strict Track adopted sparse receipt")
	}
	if err := c.ConfirmReceipt("datasource", name, "different-project", receipt); err == nil {
		t.Fatal("receipt bound to another parent request")
	}
	if err := c.ConfirmReceipt("datasource", name, "project-one", receipt); err != nil {
		t.Fatal(err)
	}
	saved, err := LoadJournal(j.path, j.Target)
	if err != nil {
		t.Fatal(err)
	}
	confirmed := saved.Entries[0].Object
	if confirmed == nil || confirmed.ID != "announced-id" || confirmed.Name != "" || confirmed.ProjectID != "" {
		t.Fatal("sparse receipt lost exact identity or invented independently unverified fields")
	}
	if err := c.ConfirmReceipt("datasource", name, "project-one", Object{Kind: "datasource", ID: "different-id"}); err == nil {
		t.Fatal("second receipt silently replaced known authoritative identity")
	}
	if err := c.ConfirmReceipt("datasource", name, "project-one", Object{Kind: "workbook", ID: "announced-id"}); err == nil {
		t.Fatal("receipt changed authoritative resource kind")
	}
	if err := c.Track(Object{Kind: "datasource", ID: "announced-id", Name: name, ProjectID: "project-one"}); err != nil {
		t.Fatalf("exact receipt could not be independently hydrated: %v", err)
	}
}

func TestPendingUnnamedMetricPreventsParentDeletion(t *testing.T) {
	j := testJournal(t)
	own(t, j, Object{Kind: "project", ID: "parent", Name: "tadx-live-safety-parent"})
	if err := j.AddIntent("pulse.metric", "tadx-live-safety-pending", "definition-one"); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(500) }))
	defer server.Close()
	if err := j.Cleanup(t.Context(), observationClient(server)); ErrorStatus(err) != InconclusiveStatus {
		t.Fatalf("pending unnamed metric did not block cleanup: %v", err)
	}
	if requests != 0 {
		t.Fatal("cleanup touched remote resources despite unresolved metric identity")
	}
	if j.Entries[0].Done {
		t.Fatal("parent was marked removed while unnamed metric remained")
	}
}

func TestActiveAcceptedJobPreventsCleanupEvenWithZeroFinishCode(t *testing.T) {
	j := testJournal(t)
	own(t, j, Object{Kind: "group", ID: "owned", Name: "tadx-live-safety-group"})
	j.Jobs = []string{"job-one"}
	deletes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deletes++
			w.WriteHeader(204)
			return
		}
		fmt.Fprint(w, `{"job":{"id":"job-one","type":"RefreshExtract","progress":"25","finishCode":"0"}}`)
	}))
	defer server.Close()
	if err := j.Cleanup(t.Context(), observationClient(server)); ErrorStatus(err) != InconclusiveStatus {
		t.Fatalf("active accepted job did not block cleanup: %v", err)
	}
	if deletes != 0 {
		t.Fatal("resources deleted while accepted job still active")
	}
}

func TestAnotherCompletedJobCannotAuthorizeAcceptedJobCleanup(t *testing.T) {
	j := testJournal(t)
	own(t, j, Object{Kind: "group", ID: "owned", Name: "tadx-live-safety-group"})
	j.Jobs = []string{"accepted-job"}
	deletes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deletes++
			w.WriteHeader(204)
			return
		}
		fmt.Fprint(w, `{"job":{"id":"another-job","type":"PublishDatasource","progress":"100","finishCode":"0"}}`)
	}))
	defer server.Close()
	if err := j.Cleanup(t.Context(), observationClient(server)); err == nil {
		t.Fatal("another terminal job authorized cleanup of accepted job resources")
	}
	if deletes != 0 {
		t.Fatal("resource deletion followed unrelated job observation")
	}
}

func TestRestorationDeleteAlreadyAbsentIsIdempotent(t *testing.T) {
	j := testJournal(t)
	if err := j.Restore("DELETE", "/api/3.29/sites/site-one/groups/group-one/users/user-one", nil); err != nil {
		t.Fatal(err)
	}
	deletes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("unexpected restore method %s", r.Method)
		}
		deletes++
		w.WriteHeader(404)
	}))
	defer server.Close()
	if err := j.Cleanup(t.Context(), observationClient(server)); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 || !j.Entries[0].Done {
		t.Fatal("authoritative DELETE404 was not durably restored")
	}
}

func TestFailedChildDeletePreventsParentCascade(t *testing.T) {
	j := testJournal(t)
	own(t, j, Object{Kind: "project", ID: "parent", Name: "tadx-live-safety-parent"})
	own(t, j, Object{Kind: "workbook", ID: "child", Name: "tadx-live-safety-child", ProjectID: "parent"})
	parentDelete := false
	childDelete := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "DELETE" && r.URL.Path == "/api/3.29/sites/site-one/workbooks/child":
			childDelete = true
			w.WriteHeader(503)
		case r.Method == "GET" && r.URL.Path == "/api/3.29/sites/site-one/workbooks/child":
			fmt.Fprint(w, `{"workbook":{"id":"child","name":"tadx-live-safety-child","project":{"id":"parent"}}}`)
		case r.Method == "DELETE":
			parentDelete = true
			w.WriteHeader(204)
		default:
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	if err := j.Cleanup(t.Context(), observationClient(server)); err == nil {
		t.Fatal("failed child cleanup reported success")
	}
	if !childDelete || parentDelete || j.Entries[0].Done {
		t.Fatal("parent cascade continued after failed child removal")
	}
}

func TestRestorationRejectsEncodedTraversalAndOtherSites(t *testing.T) {
	for _, path := range []string{"/api/3.29/sites/site-one/../groups/x", "/api/3.29/sites/site-one/%2e%2e/groups/x", "/api/3.29/sites/site-one/%252e%252e/groups/x", "/api/3.29/sites/site-one/groups/%2fother", "/api/3.29/sites/site-one/groups/x?token=value", "https://example.test/api/x", "/api/3.29/sites/site-one/groups/x#fragment"} {
		t.Run(path, func(t *testing.T) {
			if err := testJournal(t).Restore("PUT", path, map[string]any{}); err == nil {
				t.Fatalf("unsafe restoration path accepted: %s", path)
			}
		})
	}
	c := &Case{Journal: testJournal(t), API: &Client{Version: "3.29", SiteID: "site-one"}}
	if err := c.RestoreRequest("PUT", "/api/3.29/sites/another-site/groups/x", map[string]any{}); err == nil {
		t.Fatal("cross-site restoration request accepted")
	}
	if err := c.RestoreRequest("PUT", "/api/3.29/sites/site-one/groups/x", map[string]any{}); err != nil {
		t.Fatalf("same-site restoration rejected: %v", err)
	}
}

func TestAcceptedLocalWorkerBlocksCleanupUntilObservedTerminal(t *testing.T) {
	for _, state := range []struct {
		name, document string
		allowed        bool
	}{
		{"alive", `{"operation_id":"worker-one","tracking":{"worker_alive":true,"phase":"running"}}`, false},
		{"unknown", `{"operation_id":"worker-one","tracking":{"phase":"completed"}}`, false},
		{"wrong-identity", `{"operation_id":"other","tracking":{"worker_alive":false,"phase":"completed"}}`, false},
		{"completed", `{"operation":"content.datasource.publish","operation_id":"worker-one","tracking":{"worker_alive":false,"phase":"completed"}}`, true},
		{"unresolved", `{"operation_id":"worker-one","tracking":{"worker_alive":false,"phase":"running"}}`, false},
	} {
		t.Run(state.name, func(t *testing.T) {
			c := helperCase(t, state.document, 0)
			c.Journal = testJournal(t)
			own(t, c.Journal, Object{Kind: "group", ID: "owned", Name: "tadx-live-safety-group"})
			if err := c.AcceptOperation("worker-one"); err != nil {
				t.Fatal(err)
			}
			deletes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "DELETE" {
					deletes++
					w.WriteHeader(204)
					return
				}
				fmt.Fprint(w, `{"groups":{"group":[]},"pagination":{"totalAvailable":"0"}}`)
			}))
			defer server.Close()
			c.API = observationClient(server)
			err := c.Cleanup(t.Context())
			if state.allowed {
				if err != nil || deletes != 1 {
					t.Fatalf("confirmed terminal operation could not clean up: %v, deletes %d", err, deletes)
				}
				if !validOperationClosure(c.Journal.OperationClosures["worker-one"]) {
					t.Fatal("terminal no-worker observation was not durably recorded")
				}
			} else {
				if ErrorStatus(err) != InconclusiveStatus || deletes != 0 {
					t.Fatalf("unresolved worker allowed cleanup: %v, deletes %d", err, deletes)
				}
			}
		})
	}
}

func TestTerminalOperationClosureSurvivesPurgedLocalState(t *testing.T) {
	c := helperCase(t, `{"operation_id":"worker-one","tracking":{"worker_alive":false,"phase":"completed"}}`, 0)
	c.Journal = testJournal(t)
	if err := c.AcceptOperation("worker-one"); err != nil {
		t.Fatal(err)
	}
	if err := c.GuardOperations(t.Context()); err != nil {
		t.Fatalf("terminal operation was not closed: %v", err)
	}
	loaded, err := LoadJournal(c.Journal.path, c.Journal.Target)
	if err != nil {
		t.Fatal(err)
	}
	resumed := &Case{Journal: loaded, Binary: filepath.Join(c.RunDir, "purged-state-candidate.exe"), RunDir: c.RunDir}
	if err := resumed.GuardOperations(t.Context()); err != nil {
		t.Fatalf("persisted closure did not permit later guard without local worker state: %v", err)
	}
}

func TestUnknownTransferStillBlocksWithOperationClosure(t *testing.T) {
	c := helperCase(t, `{"operation_id":"worker-one","tracking":{"worker_alive":false,"phase":"completed"}}`, 0)
	c.Journal = testJournal(t)
	if err := c.AcceptOperation("worker-one"); err != nil {
		t.Fatal(err)
	}
	if err := c.GuardOperations(t.Context()); err != nil {
		t.Fatal(err)
	}
	c.Journal.TransferPending = true
	if err := c.Journal.save(); err != nil {
		t.Fatal(err)
	}
	c.Binary = filepath.Join(c.RunDir, "purged-state-candidate.exe")
	if err := c.GuardOperations(t.Context()); ErrorStatus(err) != InconclusiveStatus {
		t.Fatalf("closure evidence bypassed unknown transfer guard: %v", err)
	}
}

func TestNewAcceptanceInvalidatesPriorOperationClosure(t *testing.T) {
	c := helperCase(t, `{"operation_id":"worker-one","tracking":{"worker_alive":false,"phase":"completed"}}`, 0)
	c.Journal = testJournal(t)
	if err := c.AcceptOperation("worker-one"); err != nil {
		t.Fatal(err)
	}
	if err := c.GuardOperations(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := c.AcceptOperation("worker-one"); err != nil {
		t.Fatal(err)
	}
	if _, exists := c.Journal.OperationClosures["worker-one"]; exists {
		t.Fatal("new acceptance retained stale closure evidence")
	}
	c.Binary = filepath.Join(c.RunDir, "purged-state-candidate.exe")
	if err := c.GuardOperations(t.Context()); ErrorStatus(err) != InconclusiveStatus {
		t.Fatalf("newly accepted operation reused stale closure evidence: %v", err)
	}
}
