package datasource_test

import (
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"testing"
)

func TestPublishCompactExecutionProjectionKeepsOutcomeContextWithoutPlan(t *testing.T) {
	value := datasourceops.PublishOutput{
		Plan: datasourceops.PublishPlan{
			Mode:         "execute",
			Operation:    "datasource.publish",
			Workspace:    "analytics",
			ArtifactPath: "artifacts/datasource/Sales--id",
			Target:       datasourceops.PublishTarget{Environment: "production", Site: "marketing", ProjectLUID: "project-1", ProjectPath: "Ops"},
		},
		Result: &datasourceops.PublishResult{Status: "succeeded", DatasourceLUID: "ds-1", DatasourceName: "Sales", ProjectLUID: "project-1", ReceiptPath: "jobs/receipt-1.json", Verification: "confirmed"},
	}

	compact := value.CompactOutput().(datasourceops.PublishCompactResult)
	if compact.Plan != nil || compact.Status != "succeeded" || compact.Environment != "production" || compact.Site != "marketing" || compact.ProjectPath != "Ops" || compact.Workspace != "analytics" || compact.ArtifactPath == "" {
		t.Fatalf("compact execution context = %#v", compact)
	}
	if compact.Result == nil || compact.Result.DatasourceLUID != "ds-1" || compact.Result.DatasourceName != "Sales" || compact.Result.ProjectLUID != "project-1" || compact.Result.ReceiptPath == "" {
		t.Fatalf("compact execution result = %#v", compact.Result)
	}
}

func TestPublishCompactPreviewProjectionRetainsPlan(t *testing.T) {
	value := datasourceops.PublishOutput{Plan: datasourceops.PublishPlan{Mode: "preview", Operation: "datasource.publish", ArtifactPath: "artifacts/datasource/Sales"}}
	compact := value.CompactOutput().(datasourceops.PublishCompactResult)
	if compact.Plan == nil || compact.Plan.Mode != "preview" || compact.Status != "" {
		t.Fatalf("compact preview = %#v", compact)
	}
}
