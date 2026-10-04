package job

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/value"
)

func recoveryFixture(operationDirectory, receiptDirectory string) recovery {
	return recovery{ports: RecoveryPorts{
		OperationStore:    func() (operationrun.Store, error) { return operationrun.Store{Directory: operationDirectory}, nil },
		ReceiptStore:      func() (jobmonitor.Store, error) { return jobmonitor.Store{Directory: receiptDirectory}, nil },
		SupportsOperation: func(string) bool { return true },
		Open: func(context.Context, string) (RecoverySession, error) {
			return RecoverySession{}, errors.New("unexpected native recovery request")
		},
	}}
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
			intent := operationrun.ReceiptIntent{ID: receipt.ReceiptID, Scope: jobmonitor.ReceiptScope(receipt), RegisteredAt: registered}
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
			runtime := recoveryFixture(operationDirectory, receiptDirectory)
			recovered, warnings := runtime.recoverReceiptLinks(t.Context(), store, record)
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
						ID: receipt.ReceiptID, Scope: jobmonitor.ReceiptScope(receipt), RegisteredAt: receipt.AcceptedAt.Add(-time.Second),
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
				runtime := recoveryFixture(store.Directory, receiptStore.Directory)
				recovered, warnings := runtime.recoverReceiptLinks(t.Context(), store, record)
				if test.linked && (len(recovered.ReceiptPaths) != 1 || recovered.ReceiptPaths[0] != path || len(warnings) != 0) {
					t.Fatalf("synchronous receipt not linked: paths=%v warnings=%v", recovered.ReceiptPaths, warnings)
				}
				if !test.linked && (len(recovered.ReceiptPaths) != 0 || len(warnings) != 1) {
					t.Fatalf("invalid synchronous receipt was linked: paths=%v warnings=%v", recovered.ReceiptPaths, warnings)
				}
				if test.linked && test.status == "unknown" {
					inspected, err := runtime.inspectOperation(t.Context(), InspectInput{OperationID: record.ID})
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
