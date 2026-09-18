package inspect_test

import (
	"context"
	"encoding/json"
	"testing"

	jobinspect "github.com/ahillspace/tadx/actions/job/inspect"
	"github.com/ahillspace/tadx/internal/value"
)

type source struct {
	result jobinspect.Result
}

func (s source) Inspect(context.Context, jobinspect.Input) (jobinspect.Result, error) {
	return s.result, nil
}

func TestValidateInputRequiresExactlyOneSelector(t *testing.T) {
	cases := []struct {
		name  string
		input jobinspect.Input
		want  bool
	}{
		{name: "job", input: jobinspect.Input{ID: "job-1"}, want: true},
		{name: "operation", input: jobinspect.Input{OperationID: "run-1"}, want: true},
		{name: "none", input: jobinspect.Input{}, want: false},
		{name: "both", input: jobinspect.Input{ID: "job-1", OperationID: "run-1"}, want: false},
		{name: "operation whitespace", input: jobinspect.Input{OperationID: " run-1"}, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := jobinspect.ValidateInput(test.input)
			if (err == nil) != test.want {
				t.Fatalf("ValidateInput(%+v) error = %v, want valid=%t", test.input, err, test.want)
			}
		})
	}
}

func TestExecuteProjectsLocalOperationWithoutRemoteJob(t *testing.T) {
	view := &jobinspect.OperationView{ID: "run-1", Phase: "running", Status: "running", Alive: true}
	action := jobinspect.New(source{result: jobinspect.Result{Operation: view}})
	output, err := action.Execute(context.Background(), jobinspect.Input{OperationID: "run-1"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.Status != "running" || output.Operation != view || output.Job != (value.JobStatus{}) {
		t.Fatalf("Execute() output = %#v", output)
	}
}

func TestExecuteRejectsMismatchedLocalOperation(t *testing.T) {
	action := jobinspect.New(source{result: jobinspect.Result{Operation: &jobinspect.OperationView{ID: "run-other", Phase: "completed"}}})
	if _, err := action.Execute(context.Background(), jobinspect.Input{OperationID: "run-1"}); err == nil {
		t.Fatal("Execute() error = nil, want identity error")
	}
}

func TestOperationOutputProjectsCompactAndFullSnapshots(t *testing.T) {
	view := &jobinspect.OperationView{ID: "run-1", Phase: "completed", Status: "succeeded", Snapshot: map[string]any{"status": "succeeded"}, FullSnapshot: map[string]any{"status": "succeeded", "diagnostics": "details"}}
	action := jobinspect.New(source{result: jobinspect.Result{Operation: view}})
	output, err := action.Execute(context.Background(), jobinspect.Input{OperationID: "run-1"})
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
