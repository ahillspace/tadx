package publish_test

import (
	"bytes"
	"strings"
	"testing"

	publish "github.com/ahillspace/tadx/actions/workbook/publish"
	"github.com/ahillspace/tadx/internal/output"
)

func TestCompactExecutionProjectionKeepsOutcomeContextWithoutPlan(t *testing.T) {
	value := publish.Output{
		Plan: publish.Plan{
			Mode:         "execute",
			Operation:    "workbook.publish",
			Workspace:    "analytics",
			ArtifactPath: "artifacts/workbook/Sales--id",
			Target:       publish.Target{Environment: "production", Site: "marketing", ProjectLUID: "project-1", ProjectPath: "Ops"},
			Warnings:     []string{"A significant portability warning."},
		},
		Result: &publish.Result{
			Status:       "succeeded",
			WorkbookLUID: "wb-1",
			WorkbookName: "Sales",
			ProjectLUID:  "project-1",
			JobID:        "job-1",
			ReceiptPath:  "jobs/job-1.json",
			Verification: "destination_unavailable",
		},
	}

	compact := value.CompactOutput().(publish.CompactResult)
	if compact.Plan != nil || compact.Status != "succeeded" || compact.Environment != "production" || compact.Site != "marketing" || compact.ProjectPath != "Ops" || compact.Workspace != "analytics" || compact.ArtifactPath == "" {
		t.Fatalf("compact execution context = %#v", compact)
	}
	if compact.Result == nil || compact.Result.WorkbookLUID != "wb-1" || compact.Result.WorkbookName != "Sales" || compact.Result.ProjectLUID != "project-1" || compact.Result.ReceiptPath == "" || compact.Result.Verification != "destination_unavailable" {
		t.Fatalf("compact execution result = %#v", compact.Result)
	}
}

func TestCompactPreviewProjectionRetainsPlan(t *testing.T) {
	value := publish.Output{Plan: publish.Plan{Mode: "preview", Operation: "workbook.publish", ArtifactPath: "artifacts/workbook/Sales"}}
	compact := value.CompactOutput().(publish.CompactResult)
	if compact.Plan == nil || compact.Plan.Mode != "preview" || compact.Status != "" {
		t.Fatalf("compact preview = %#v", compact)
	}
}

func TestCompactExecutionProjectionIsSharedByTOONAndJSON(t *testing.T) {
	value := publish.Output{
		Plan:   publish.Plan{Mode: "execute", Operation: "workbook.publish", ArtifactPath: "artifacts/workbook/Sales", Target: publish.Target{Environment: "production", Site: "marketing"}},
		Result: &publish.Result{Status: "succeeded", WorkbookLUID: "wb-1", WorkbookName: "Sales", ProjectLUID: "project-1"},
	}
	var toon, json bytes.Buffer
	if err := output.Render(&toon, value); err != nil {
		t.Fatal(err)
	}
	if err := output.RenderWithOptions(&json, value, output.Options{JSON: true}); err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{toon.String(), json.String()} {
		if strings.Contains(rendered, "plan") || !strings.Contains(rendered, "production") || !strings.Contains(rendered, "wb-1") {
			t.Fatalf("compact projection = %q", rendered)
		}
	}
}
