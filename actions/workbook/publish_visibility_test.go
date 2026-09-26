package workbook_test

import (
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"strings"
	"testing"
)

func TestPublishSuccessfulJobWithDelayedIndexIsNotReportedAsFailed(t *testing.T) {
	action := workbookops.NewPublish(nil, publishResolver{}, nil)
	out, err := action.Complete(t.Context(), workbookops.PublishOutput{
		Plan:   workbookops.PublishPlan{Mode: "execute", WorkbookName: "Finance", Target: workbookops.PublishTarget{Environment: "dev", ProjectLUID: "project-1"}},
		Result: &workbookops.PublishResult{Status: "succeeded", JobID: "job-1"},
	})
	if err != nil || out.Result.Status != "succeeded" || out.Result.Verification != "destination_pending" || out.Result.WorkbookLUID != "" {
		t.Fatalf("result=%+v err=%v", out.Result, err)
	}
	if !strings.Contains(strings.Join(out.Help, " "), "job inspect") {
		t.Fatalf("missing exact recovery: %v", out.Help)
	}
}
