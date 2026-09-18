package publish_test

import (
	"strings"
	"testing"

	"github.com/ahillspace/tadx/actions/workbook/publish"
)

func TestSuccessfulJobWithDelayedIndexIsNotReportedAsFailed(t *testing.T) {
	action := publish.New(nil, resolver{}, nil)
	out, err := action.Complete(t.Context(), publish.Output{
		Plan:   publish.Plan{Mode: "execute", WorkbookName: "Finance", Target: publish.Target{Environment: "dev", ProjectLUID: "project-1"}},
		Result: &publish.Result{Status: "succeeded", JobID: "job-1"},
	})
	if err != nil || out.Result.Status != "succeeded" || out.Result.Verification != "destination_pending" || out.Result.WorkbookLUID != "" {
		t.Fatalf("result=%+v err=%v", out.Result, err)
	}
	if !strings.Contains(strings.Join(out.Help, " "), "job inspect") {
		t.Fatalf("missing exact recovery: %v", out.Help)
	}
}
