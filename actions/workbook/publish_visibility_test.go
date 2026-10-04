package workbook

import (
	"strings"
	"testing"
)

func TestPublishSuccessfulJobWithDelayedIndexIsNotReportedAsFailed(t *testing.T) {
	action := newPublisher(nil, publishResolver{}, nil)
	out, err := action.Complete(t.Context(), PublishOutput{
		Plan:   PublishPlan{Mode: "execute", WorkbookName: "Finance", Target: PublishTarget{Environment: "dev", ProjectLUID: "project-1"}},
		Result: &PublishResult{Status: "succeeded", JobID: "job-1"},
	})
	if err != nil || out.Result.Status != "succeeded" || out.Result.Verification != "destination_pending" || out.Result.WorkbookLUID != "" {
		t.Fatalf("result=%+v err=%v", out.Result, err)
	}
	if !strings.Contains(strings.Join(out.Help, " "), "job inspect") {
		t.Fatalf("missing exact recovery: %v", out.Help)
	}
}
