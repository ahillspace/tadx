package update_test

import (
	"encoding/json"
	"strings"
	"testing"

	workbookupdate "github.com/ahillspace/tadx/actions/workbook/update"
)

func TestUpdateReceiptProjectsConfirmedDescriptionWithoutRequestID(t *testing.T) {
	for _, description := range []string{"Confirmed", ""} {
		output := workbookupdate.Output{Result: &workbookupdate.Result{
			Status:           "succeeded",
			WorkbookLUID:     "wb-1",
			Description:      new(description),
			EvidenceSource:   "tableau_update_response",
			TableauRequestID: "request-update",
		}}
		compact, err := json.Marshal(output.CompactOutput())
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"description":` + string(mustJSON(t, description)), `"evidence_source":"tableau_update_response"`} {
			if !strings.Contains(string(compact), want) {
				t.Errorf("compact receipt missing %q: %s", want, compact)
			}
		}
		if strings.Contains(string(compact), "request-update") {
			t.Fatalf("compact receipt exposed request ID: %s", compact)
		}

		full, err := json.Marshal(output.FullOutput())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(full), `"tableau_request_id":"request-update"`) {
			t.Fatalf("full receipt omitted request ID: %s", full)
		}
	}
}

func TestUpdateReceiptOmitsUnverifiedDescription(t *testing.T) {
	output := workbookupdate.Output{Result: &workbookupdate.Result{Status: "unknown", WorkbookLUID: "wb-1", TableauRequestID: "request-update"}}
	for _, projection := range []any{output.CompactOutput(), output.FullOutput()} {
		encoded, err := json.Marshal(projection)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "description") || strings.Contains(string(encoded), "evidence_source") {
			t.Fatalf("unverified receipt exposed evidence: %s", encoded)
		}
	}
}

func mustJSON(t *testing.T, value string) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
