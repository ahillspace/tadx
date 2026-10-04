package app

import (
	"encoding/json"
	"testing"

	jobactions "github.com/ahillspace/tadx/actions/job"
	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestPublicationBatchReconciliationClassifiesAllFailuresAndMixedOutcomes(t *testing.T) {
	for _, test := range []struct {
		name         string
		itemStatuses []string
		wantStatus   string
	}{
		{name: "all failed", itemStatuses: []string{"failed", "failed"}, wantStatus: "failed"},
		{name: "mixed failure and success", itemStatuses: []string{"failed", "succeeded"}, wantStatus: "partial_failure"},
	} {
		t.Run(test.name, func(t *testing.T) {
			operationDirectory := t.TempDir()
			store := operationrun.Store{Directory: operationDirectory}
			record, err := store.Create(operationrun.Request{Operation: "workbook.publish"})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			receiptPaths := []string{"receipt-a.json", "receipt-b.json"}
			snapshot := json.RawMessage(`{"status":"partial_failure","items":[{"status":"pending","receipt_path":"receipt-a.json"},{"status":"pending","receipt_path":"receipt-b.json"}]}`)
			record, err = store.Update(record.ID, func(current *operationrun.Record) error {
				current.Phase = operationrun.PhaseRemotePending
				current.ReceiptPaths = receiptPaths
				current.CompactResult, current.FullResult = snapshot, snapshot
				return nil
			})
			if err != nil {
				t.Fatalf("Update() error = %v", err)
			}

			items := []jobactions.OperationItem{
				{Status: test.itemStatuses[0], ReceiptPath: receiptPaths[0], JobID: "job-a"},
				{Status: test.itemStatuses[1], ReceiptPath: receiptPaths[1], JobID: "job-b"},
			}
			runtime := &runtimeDependencies{operationDirectory: operationDirectory}
			updated, warnings := runtime.reconcilePublicationReceipts(t.Context(), record, items)
			if len(warnings) != 0 {
				t.Fatalf("reconcile warnings = %v", warnings)
			}
			if updated == nil {
				t.Fatal("reconcile returned no updated record")
			}
			if updated.Phase != operationrun.PhaseFailed {
				t.Fatalf("reconciled phase = %q, want failed", updated.Phase)
			}
			var result map[string]any
			if err := json.Unmarshal(updated.FullResult, &result); err != nil {
				t.Fatalf("decode reconciled result: %v", err)
			}
			if result["status"] != test.wantStatus {
				t.Fatalf("reconciled status = %#v, want %q", result["status"], test.wantStatus)
			}
		})
	}
}
