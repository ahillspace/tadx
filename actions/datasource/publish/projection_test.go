package publish_test

import (
	"testing"

	publish "github.com/ahillspace/tadx/actions/datasource/publish"
)

func TestCompactExecutionProjectionKeepsOutcomeContextWithoutPlan(t *testing.T) {
	value := publish.Output{
		Plan: publish.Plan{
			Mode:         "execute",
			Operation:    "datasource.publish",
			Workspace:    "analytics",
			ArtifactPath: "artifacts/datasource/Sales--id",
			Target:       publish.Target{Environment: "production", Site: "marketing", ProjectLUID: "project-1", ProjectPath: "Ops"},
		},
		Result: &publish.Result{Status: "pending", DatasourceLUID: "ds-1", DatasourceName: "Sales", ProjectLUID: "project-1", JobID: "job-1", ReceiptPath: "jobs/job-1.json", Verification: "accepted"},
	}

	compact := value.CompactOutput().(publish.CompactResult)
	if compact.Plan != nil || compact.Status != "pending" || compact.Environment != "production" || compact.Site != "marketing" || compact.ProjectPath != "Ops" || compact.Workspace != "analytics" || compact.ArtifactPath == "" {
		t.Fatalf("compact execution context = %#v", compact)
	}
	if compact.Result == nil || compact.Result.DatasourceLUID != "ds-1" || compact.Result.DatasourceName != "Sales" || compact.Result.ProjectLUID != "project-1" || compact.Result.ReceiptPath == "" {
		t.Fatalf("compact execution result = %#v", compact.Result)
	}
}

func TestCompactPreviewProjectionRetainsPlan(t *testing.T) {
	value := publish.Output{Plan: publish.Plan{Mode: "preview", Operation: "datasource.publish", ArtifactPath: "artifacts/datasource/Sales"}}
	compact := value.CompactOutput().(publish.CompactResult)
	if compact.Plan == nil || compact.Plan.Mode != "preview" || compact.Status != "" {
		t.Fatalf("compact preview = %#v", compact)
	}
}
