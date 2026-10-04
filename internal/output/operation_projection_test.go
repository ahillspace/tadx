package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

func TestFullPublicationBatchReconciliationCrossesResultWrappers(t *testing.T) {
	data := []byte(`{"status":"running","operation":"workbook.publish","items":[{"status":"pending","result":{"plan":{"operation":"workbook.publish"},"result":{"status":"pending","receipt_path":"jobs/a.json","tableau_job_id":"a"}}}]}`)
	items := []value.OperationItem{{Status: "succeeded", ResourceID: "published-a", ReceiptPath: "jobs/a.json", JobID: "a"}}
	merged := MergeOperationResult(data, items, "succeeded", "workbook.publish")
	if OperationHasPending(merged) || !strings.Contains(string(merged), `"succeeded":1`) {
		t.Fatalf("terminal receipt left pending wrappers: %s", merged)
	}
}

func TestPublicationBatchSnapshotKeepsUsefulDetailsWithoutRepeatedEnvelopes(t *testing.T) {
	record := value.OperationRecord{ID: "run-test", Operation: "workbook.publish", Phase: "completed",
		CompactResult: json.RawMessage(`{"status":"succeeded","total":2,"items":[{"selector":"id=a","status":"succeeded","result":{"environment":"dev","site":"team","workspace":"work","project_path":"Reports","artifact_path":"artifacts/workbook/a","help":[],"details":"--full","result":{"status":"succeeded","workbook_luid":"a-new","receipt_path":"jobs/a.json"}}},{"selector":"id=b","status":"succeeded","result":{"environment":"dev","site":"team","workspace":"work","project_path":"Reports","artifact_path":"artifacts/workbook/b","help":[],"details":"--full","result":{"status":"succeeded","workbook_luid":"b-new","receipt_path":"jobs/b.json"}}}]}`)}
	data, err := json.Marshal(OperationSnapshot(record, false, false))
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
