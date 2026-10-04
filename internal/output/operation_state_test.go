package output

import (
	"encoding/json"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

func TestOperationStatePredicatesKeepDistinctRecoveryMeanings(t *testing.T) {
	for _, test := range []struct {
		status                      string
		unfinished, pending, failed bool
	}{
		{"accepted", true, false, false},
		{"pending", true, true, false},
		{"running", true, true, false},
		{"queued", true, true, false},
		{"unknown", false, true, false},
		{"destination_pending", false, true, false},
		{"succeeded", false, false, false},
		{"failed", false, false, true},
		{"partial_failure", false, false, true},
		{"skipped", false, false, true},
	} {
		t.Run(test.status, func(t *testing.T) {
			data := json.RawMessage(`{"items":[{"result":{"status":"` + test.status + `"}}]}`)
			if OperationContainsUnfinished(data) != test.unfinished || OperationHasPending(data) != test.pending || OperationHasFailure(data) != test.failed {
				t.Fatalf("state predicates changed for %s", data)
			}
		})
	}
}

func TestOperationReconciliationPreservesUnparseableEvidence(t *testing.T) {
	for _, data := range [][]byte{nil, {}, []byte("{")} {
		merged := MergeOperationResult(data, nil, "succeeded", "workbook.publish")
		if string(merged) != string(data) || (data == nil) != (merged == nil) {
			t.Fatalf("data=%v merged=%v", data, merged)
		}
	}
}

func TestOperationRunDoesNotExposeRawRecordsWithoutProjection(t *testing.T) {
	data, err := json.Marshal(OperationRun{Record: value.OperationRecord{ID: "run-1", FullResult: json.RawMessage(`{"status":"failed"}`)}, Stopped: true})
	if err != nil || string(data) != "{}" {
		t.Fatalf("data=%s err=%v", data, err)
	}
}
