package job_test

import (
	"context"
	"encoding/json"
	"testing"

	jobactions "github.com/ahillspace/tadx/actions/job"
	"github.com/ahillspace/tadx/internal/value"
)

func TestValidateInputRequiresExactlyOneSelector(t *testing.T) {
	cases := []struct {
		name  string
		input jobactions.InspectInput
		want  bool
	}{
		{name: "job", input: jobactions.InspectInput{ID: "job-1"}, want: true},
		{name: "operation", input: jobactions.InspectInput{OperationID: "run-1"}, want: true},
		{name: "none", input: jobactions.InspectInput{}, want: false},
		{name: "both", input: jobactions.InspectInput{ID: "job-1", OperationID: "run-1"}, want: false},
		{name: "operation whitespace", input: jobactions.InspectInput{OperationID: " run-1"}, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := jobactions.ValidateInspectInput(test.input)
			if (err == nil) != test.want {
				t.Fatalf("ValidateInput(%+v) error = %v, want valid=%t", test.input, err, test.want)
			}
		})
	}
}

func TestExecuteProjectsLocalOperationWithoutRemoteJob(t *testing.T) {
	view := &jobactions.OperationView{ID: "run-1", Phase: "running", Status: "running", Alive: true}
	output, err := jobactions.Inspect(t.Context(), func(context.Context, jobactions.InspectInput) (jobactions.InspectResult, error) {
		return jobactions.InspectResult{Operation: view}, nil
	}, jobactions.InspectInput{OperationID: "run-1"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.Status != "running" || output.Operation != view || output.Job != (value.JobStatus{}) {
		t.Fatalf("Execute() output = %#v", output)
	}
}

func TestOperationOutputProjectsCompactAndFullSnapshots(t *testing.T) {
	view := &jobactions.OperationView{ID: "run-1", Phase: "completed", Status: "succeeded", Snapshot: map[string]any{"status": "succeeded"}, FullSnapshot: map[string]any{"status": "succeeded", "diagnostics": "details"}}
	output, err := jobactions.Inspect(t.Context(), func(context.Context, jobactions.InspectInput) (jobactions.InspectResult, error) {
		return jobactions.InspectResult{Operation: view}, nil
	}, jobactions.InspectInput{OperationID: "run-1"})
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
