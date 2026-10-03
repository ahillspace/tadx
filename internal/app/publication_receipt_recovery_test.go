package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jobactions "github.com/ahillspace/tadx/actions/job"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/value"
)

func TestOperationInspectKeepsBatchIntentOrderAfterPartialReceiptRecovery(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected remote request", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	runtime, _ := datasourceLifecycleRuntime(t, server)
	configPath := runtime.configPath
	operationDirectory, receiptDirectory := t.TempDir(), t.TempDir()
	store := operationrun.Store{Directory: operationDirectory}
	record, err := store.Create(operationrun.Request{Operation: "workbook.publish", ConfigPath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	registered := time.Now().UTC()
	receiptStore := jobmonitor.Store{Directory: receiptDirectory}
	var intents []operationrun.ReceiptIntent
	var paths []string
	for index, resourceID := range []string{"workbook-first", "workbook-second"} {
		receipt := jobmonitor.Receipt{
			ReceiptID: resourceID + "-receipt", OperationID: record.ID, Version: 1,
			Operation: record.Operation, Environment: "production", Server: server.URL,
			Site: "team-site", SiteID: "site-id", ConfigPath: configPath,
			SourcePath: resourceID + ".twb", ProjectID: "project-1", Name: resourceID,
			CoordinationKey: "opaque", AcceptedAt: registered.Add(time.Duration(index+1) * time.Second),
			Observation: value.JobStatus{Status: "unknown", ResourceID: resourceID},
		}
		intents = append(intents, operationrun.ReceiptIntent{ID: receipt.ReceiptID, Scope: publicationReceiptScope(receipt), RegisteredAt: registered})
		path, err := receiptStore.Save(t.Context(), receipt)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	_, err = store.Update(record.ID, func(current *operationrun.Record) error {
		current.Phase = operationrun.PhaseRunning
		current.StartedAt = registered
		current.ReceiptIntents = intents
		current.ReceiptPaths = []string{paths[1]}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	options := Options{ConfigPath: configPath, OperationDirectory: operationDirectory, JobDirectory: receiptDirectory, HTTPClient: server.Client()}
	if exit := Run(t.Context(), []string{"job", "inspect", "--operation-id", record.ID, "--json", "--full"}, &output, options); exit != 0 {
		t.Fatalf("operation inspection exit=%d output=%s", exit, output.String())
	}
	var inspection struct {
		Status string `json:"status"`
		Jobs   []struct {
			ResourceID string `json:"workbook_luid"`
		} `json:"accepted_jobs"`
	}
	if err := json.Unmarshal([]byte(output.String()), &inspection); err != nil {
		t.Fatal(err)
	}
	if inspection.Status != "interrupted" || len(inspection.Jobs) != 2 || inspection.Jobs[0].ResourceID != "workbook-first" || inspection.Jobs[1].ResourceID != "workbook-second" {
		t.Fatalf("accepted jobs lost batch order: %+v", inspection)
	}
	updated, err := store.Read(record.ID)
	if err != nil || len(updated.ReceiptPaths) != 2 || updated.ReceiptPaths[0] != paths[0] || updated.ReceiptPaths[1] != paths[1] {
		t.Fatalf("saved receipt order=%v err=%v", updated.ReceiptPaths, err)
	}
	if requests.Load() != 0 {
		t.Fatalf("operation recovery made %d remote requests", requests.Load())
	}
}

func TestRecoverPublicationReceiptLinksRequiresRegisteredExactScope(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(*jobmonitor.Receipt)
		linked  bool
		missing bool
	}{
		{name: "matching intent", linked: true},
		{name: "another operation", change: func(r *jobmonitor.Receipt) { r.OperationID = "run-other" }},
		{name: "changed project", change: func(r *jobmonitor.Receipt) { r.ProjectID = "project-other" }},
		{name: "changed source", change: func(r *jobmonitor.Receipt) { r.SourcePath = "other.twb" }},
		{name: "stale acceptance", change: func(r *jobmonitor.Receipt) { r.AcceptedAt = r.AcceptedAt.Add(-2 * time.Minute) }},
		{name: "legacy receipt", change: func(r *jobmonitor.Receipt) { r.OperationID = "" }},
		{name: "historical same target receipt", change: func(r *jobmonitor.Receipt) {
			r.ReceiptID = "older-receipt-id"
			r.OperationID = ""
		}, missing: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			operationDirectory, receiptDirectory := t.TempDir(), t.TempDir()
			store := operationrun.Store{Directory: operationDirectory}
			record, err := store.Create(operationrun.Request{Operation: "workbook.publish"})
			if err != nil {
				t.Fatal(err)
			}
			registered := time.Now().UTC().Add(-time.Minute)
			receipt := jobmonitor.Receipt{
				ReceiptID: "random-receipt-id", OperationID: record.ID, Version: 1,
				Operation: record.Operation, Environment: "production", Server: "https://example.test",
				Site: "team-site", SiteID: "site-id", ConfigPath: filepath.Join(t.TempDir(), "config.yaml"),
				SourcePath: "source.twb", ProjectID: "project-1", Name: "Sales",
				CoordinationKey: "opaque", AcceptedAt: registered.Add(time.Second),
				Observation: value.JobStatus{ID: "job-1", Type: "PublishWorkbook", Status: "pending"},
			}
			intent := operationrun.ReceiptIntent{ID: receipt.ReceiptID, Scope: publicationReceiptScope(receipt), RegisteredAt: registered}
			record, err = store.Update(record.ID, func(current *operationrun.Record) error {
				current.ReceiptIntents = append(current.ReceiptIntents, intent)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if test.change != nil {
				test.change(&receipt)
			}
			receiptStore := jobmonitor.Store{Directory: receiptDirectory}
			path, err := receiptStore.Save(t.Context(), receipt)
			if err != nil {
				t.Fatal(err)
			}
			runtime := &runtimeDependencies{jobDirectory: receiptDirectory, operationDirectory: operationDirectory}
			recovered, warnings := runtime.recoverPublicationReceiptLinks(t.Context(), store, record)
			expectedWarnings := 1
			if test.missing {
				expectedWarnings = 0
			}
			if test.linked {
				if len(recovered.ReceiptPaths) != 1 || recovered.ReceiptPaths[0] != path || len(warnings) != 0 {
					t.Fatalf("matching receipt was not linked: paths=%v warnings=%v", recovered.ReceiptPaths, warnings)
				}
			} else if len(recovered.ReceiptPaths) != 0 || len(warnings) != expectedWarnings {
				t.Fatalf("mismatched receipt was adopted: paths=%v warnings=%v", recovered.ReceiptPaths, warnings)
			}
		})
	}
}

func TestRecoverPublicationReceiptLinksRetainsSynchronousOutcomes(t *testing.T) {
	for _, operation := range []string{"workbook.publish", "datasource.publish", "flow.publish"} {
		for _, test := range []struct {
			name, status, resourceID, requestID string
			linked                              bool
		}{
			{name: "succeeded with resource", status: "succeeded", resourceID: "resource-1", linked: true},
			{name: "succeeded without resource", status: "succeeded"},
			{name: "unknown with resource and request", status: "unknown", resourceID: "resource-1", requestID: "request-1", linked: true},
			{name: "unknown with request only", status: "unknown", requestID: "request-1", linked: true},
			{name: "unknown without identifiers", status: "unknown", linked: true},
			{name: "pending without job", status: "pending", resourceID: "resource-1"},
			{name: "empty status", resourceID: "resource-1"},
		} {
			t.Run(operation+"/"+test.name, func(t *testing.T) {
				store := operationrun.Store{Directory: t.TempDir()}
				record, err := store.Create(operationrun.Request{Operation: operation})
				if err != nil {
					t.Fatal(err)
				}
				receipt := jobmonitor.Receipt{
					ReceiptID: "synchronous-receipt-id", OperationID: record.ID, Version: 1,
					Operation: record.Operation, Environment: "production", Server: "https://example.test",
					Site: "team-site", SiteID: "site-id", ConfigPath: "config.yaml",
					SourcePath: "source", ProjectID: "project-1", Name: "Published item",
					CoordinationKey: "opaque", AcceptedAt: time.Now().UTC(),
					Observation: value.JobStatus{Status: test.status, ResourceID: test.resourceID, RequestID: test.requestID},
				}
				record, err = store.Update(record.ID, func(current *operationrun.Record) error {
					current.Phase = operationrun.PhaseRunning
					current.StartedAt = receipt.AcceptedAt.Add(-time.Second)
					current.ReceiptIntents = append(current.ReceiptIntents, operationrun.ReceiptIntent{
						ID: receipt.ReceiptID, Scope: publicationReceiptScope(receipt), RegisteredAt: receipt.AcceptedAt.Add(-time.Second),
					})
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				receiptStore := jobmonitor.Store{Directory: t.TempDir()}
				path, err := receiptStore.Save(t.Context(), receipt)
				if err != nil {
					t.Fatal(err)
				}
				runtime := &runtimeDependencies{jobDirectory: receiptStore.Directory, operationDirectory: store.Directory}
				recovered, warnings := runtime.recoverPublicationReceiptLinks(t.Context(), store, record)
				if test.linked && (len(recovered.ReceiptPaths) != 1 || recovered.ReceiptPaths[0] != path || len(warnings) != 0) {
					t.Fatalf("synchronous receipt not linked: paths=%v warnings=%v", recovered.ReceiptPaths, warnings)
				}
				if !test.linked && (len(recovered.ReceiptPaths) != 0 || len(warnings) != 1) {
					t.Fatalf("invalid synchronous receipt was linked: paths=%v warnings=%v", recovered.ReceiptPaths, warnings)
				}
				if test.linked && test.status == "unknown" {
					inspected, err := runtime.inspectPublicationOperation(t.Context(), jobactions.InspectInput{OperationID: record.ID})
					if err != nil || inspected.Operation == nil || len(inspected.Operation.Items) != 1 {
						t.Fatalf("uncertain receipt inspection: result=%+v err=%v", inspected, err)
					}
					item := inspected.Operation.Items[0]
					if inspected.Operation.Status != "interrupted" || item.Status != "unknown" || item.ResourceID != test.resourceID || item.TableauRequestID != test.requestID {
						t.Fatalf("uncertain receipt identity or status changed: view=%+v item=%+v", inspected.Operation, item)
					}
				}
			})
		}
	}
}
