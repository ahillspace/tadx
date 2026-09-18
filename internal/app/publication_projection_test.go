package app

import (
	"encoding/json"
	"strings"
	"testing"

	jobinspect "github.com/ahillspace/tadx/actions/job/inspect"
	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestFullPublicationBatchReconciliationCrossesResultWrappers(t *testing.T) {
	data := []byte(`{"status":"running","operation":"workbook.publish","items":[{"status":"pending","result":{"plan":{"operation":"workbook.publish"},"result":{"status":"pending","receipt_path":"jobs/a.json","tableau_job_id":"a"}}}]}`)
	items := []jobinspect.OperationItem{{Status: "succeeded", ResourceID: "published-a", ReceiptPath: "jobs/a.json", JobID: "a"}}
	merged := mergePublicationResult(data, items, "succeeded", "workbook.publish")
	if publicationHasPending(merged) || !strings.Contains(string(merged), `"succeeded":1`) {
		t.Fatalf("terminal receipt left pending wrappers: %s", merged)
	}
}

func TestPublicationBatchSnapshotKeepsUsefulDetailsWithoutRepeatedEnvelopes(t *testing.T) {
	record := operationrun.Record{ID: "run-test", Operation: "workbook.publish", Phase: operationrun.PhaseCompleted,
		CompactResult: json.RawMessage(`{"status":"succeeded","total":2,"items":[{"selector":"id=a","status":"succeeded","result":{"environment":"dev","site":"team","workspace":"work","project_path":"Reports","artifact_path":"artifacts/workbook/a","help":[],"details":"--full","result":{"status":"succeeded","workbook_luid":"a-new","receipt_path":"jobs/a.json"}}},{"selector":"id=b","status":"succeeded","result":{"environment":"dev","site":"team","workspace":"work","project_path":"Reports","artifact_path":"artifacts/workbook/b","help":[],"details":"--full","result":{"status":"succeeded","workbook_luid":"b-new","receipt_path":"jobs/b.json"}}}]}`)}
	data, err := json.Marshal(publicationSnapshot(record, false, false))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Count(text, `"environment"`) != 1 || strings.Contains(text, `"receipt_path"`) || strings.Contains(text, `"help":[]`) || strings.Contains(text, `"result":{"result"`) {
		t.Fatalf("redundant compact output: %s", text)
	}
	for _, want := range []string{"a-new", "b-new", "artifacts/workbook/a", "artifacts/workbook/b", "run-test", "Reports"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s in %s", want, text)
		}
	}
}

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

			items := []jobinspect.OperationItem{
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
