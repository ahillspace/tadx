package job

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

func TestValidateInputRequiresExactlyOneSelector(t *testing.T) {
	cases := []struct {
		name  string
		input InspectInput
		want  bool
	}{
		{name: "job", input: InspectInput{ID: "job-1"}, want: true},
		{name: "operation", input: InspectInput{OperationID: "run-1"}, want: true},
		{name: "none", input: InspectInput{}, want: false},
		{name: "both", input: InspectInput{ID: "job-1", OperationID: "run-1"}, want: false},
		{name: "operation whitespace", input: InspectInput{OperationID: " run-1"}, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateInspectInput(test.input)
			if (err == nil) != test.want {
				t.Fatalf("ValidateInput(%+v) error = %v, want valid=%t", test.input, err, test.want)
			}
		})
	}
}

func TestExecuteProjectsLocalOperationWithoutRemoteJob(t *testing.T) {
	view := &OperationView{ID: "run-1", Phase: "running", Status: "running", Alive: true}
	output, err := inspectOutput(t.Context(), func(context.Context, InspectInput) (InspectResult, error) {
		return InspectResult{Operation: view}, nil
	}, InspectInput{OperationID: "run-1"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.Status != "running" || output.Operation != view || output.Job != (value.JobStatus{}) {
		t.Fatalf("Execute() output = %#v", output)
	}
}

func TestOperationOutputProjectsCompactAndFullSnapshots(t *testing.T) {
	view := &OperationView{ID: "run-1", Phase: "completed", Status: "succeeded", Snapshot: map[string]any{"status": "succeeded"}, FullSnapshot: map[string]any{"status": "succeeded", "diagnostics": "details"}}
	output, err := inspectOutput(t.Context(), func(context.Context, InspectInput) (InspectResult, error) {
		return InspectResult{Operation: view}, nil
	}, InspectInput{OperationID: "run-1"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var compact map[string]any
	compactData, err := json.Marshal(output.CompactOutput())
	if err != nil {
		t.Fatalf("marshal CompactOutput(): %v", err)
	}
	if err := json.Unmarshal(compactData, &compact); err != nil {
		t.Fatalf("decode CompactOutput(): %v", err)
	}
	if _, ok := compact["diagnostics"]; ok {
		t.Fatalf("compact snapshot retained diagnostics: %#v", compact)
	}
	var full map[string]any
	fullData, err := json.Marshal(output.FullOutput())
	if err != nil {
		t.Fatalf("marshal FullOutput(): %v", err)
	}
	if err := json.Unmarshal(fullData, &full); err != nil {
		t.Fatalf("decode FullOutput(): %v", err)
	}
	if full["diagnostics"] != "details" {
		t.Fatalf("full snapshot = %#v", full)
	}
}

func TestOperationRequestIDAppearsOnlyInInterruptedRecovery(t *testing.T) {
	item := OperationItem{Status: "pending", JobID: "job-1", TableauRequestID: "request-1"}
	for _, test := range []struct {
		name, status, phase string
		wantRequestID       bool
	}{
		{name: "completed", status: "succeeded", phase: "completed"},
		{name: "interrupted", status: "interrupted", phase: "running", wantRequestID: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := &OperationView{ID: "run-1", Operation: "workbook.publish", Status: test.status, Phase: test.phase, Items: []OperationItem{item}}
			output := InspectOutput{Status: test.status, Operation: view}
			projected, ok := output.FullOutput().(map[string]any)
			if !ok {
				t.Fatalf("full projection is not an object: %#v", output.FullOutput())
			}
			items, ok := projected["items"].([]map[string]any)
			if !ok || len(items) != 1 || items[0]["status"] != "pending" {
				t.Fatalf("saved item projection = %#v", projected["items"])
			}
			if _, present := items[0]["tableau_request_id"]; present {
				t.Fatalf("normal item projection widened: %#v", items[0])
			}
			accepted, present := projected["accepted_jobs"].([]map[string]any)
			if present != test.wantRequestID {
				t.Fatalf("accepted recovery presence=%t projection=%#v", present, projected)
			}
			if test.wantRequestID && (len(accepted) != 1 || accepted[0]["tableau_request_id"] != "request-1" || accepted[0]["tableau_job_id"] != "job-1" || accepted[0]["status"] != "pending") {
				t.Fatalf("uncertain recovery evidence = %#v", accepted)
			}
		})
	}
}
