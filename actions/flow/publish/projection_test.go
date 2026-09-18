package publish_test

import (
	"testing"

	publish "github.com/ahillspace/tadx/actions/flow/publish"
)

func TestCompactExecutionProjectionKeepsOutcomeContextWithoutPlan(t *testing.T) {
	value := publish.Output{
		Plan: publish.Plan{
			Mode:         "execute",
			Operation:    "flow.publish",
			Workspace:    "analytics",
			ArtifactPath: "artifacts/flow/Daily--id",
			Target:       publish.Target{Environment: "production", Site: "marketing", ProjectLUID: "project-1", ProjectPath: "Ops"},
		},
		Result: &publish.Result{Status: "succeeded", FlowLUID: "flow-1", FlowName: "Daily", ProjectLUID: "project-1", Verification: "confirmed"},
	}

	compact := value.CompactOutput().(publish.CompactResult)
	if compact.Plan != nil || compact.Status != "succeeded" || compact.Environment != "production" || compact.Site != "marketing" || compact.ProjectPath != "Ops" || compact.Workspace != "analytics" || compact.ArtifactPath == "" {
		t.Fatalf("compact execution context = %#v", compact)
	}
	if compact.Result == nil || compact.Result.FlowLUID != "flow-1" || compact.Result.FlowName != "Daily" || compact.Result.ProjectLUID != "project-1" || compact.Result.Verification != "confirmed" {
		t.Fatalf("compact execution result = %#v", compact.Result)
	}
}

func TestCompactPreviewProjectionRetainsPlan(t *testing.T) {
	value := publish.Output{Plan: publish.Plan{Mode: "preview", Operation: "flow.publish", ArtifactPath: "artifacts/flow/Daily"}}
	compact := value.CompactOutput().(publish.CompactResult)
	if compact.Plan == nil || compact.Plan.Mode != "preview" || compact.Status != "" {
		t.Fatalf("compact preview = %#v", compact)
	}
}
