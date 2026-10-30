package job_test

import (
	"context"
	"errors"
	"testing"

	jobactions "github.com/ahillspace/tadx/actions/job"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type cancelSource struct {
	result jobactions.CancelResult
	err    error
}

func (s cancelSource) Cancel(context.Context, jobactions.CancelInput) (jobactions.CancelResult, error) {
	return s.result, s.err
}

func TestExecutePreservesRequestIdentityOnUnknownConfirmation(t *testing.T) {
	status := value.JobStatus{ID: "job-1", Status: "running"}
	accepted := jobactions.CancelResult{Status: status, Environment: "dev", Site: "site", RequestID: "request-1"}
	failure := &errs.Error{ID: "job.cancel.confirmation", Outcome: errs.OutcomeUnknown}
	output, err := jobactions.Cancel(t.Context(), (cancelSource{result: accepted, err: failure}).Cancel, jobactions.CancelInput{ID: "job-1"})
	if !errors.Is(err, failure) || output.Job != status || output.RequestID != accepted.RequestID || output.Confirmed || len(output.Help) != 1 {
		t.Fatalf("output=%+v error=%v", output, err)
	}
}

func TestExecutePreviewProjectsInspectedJob(t *testing.T) {
	status := value.JobStatus{ID: "job-1", Status: "running"}
	output, err := jobactions.Cancel(t.Context(), (cancelSource{result: jobactions.CancelResult{Status: status, Environment: "dev", Site: "site"}}).Cancel, jobactions.CancelInput{ID: "job-1", Preview: true})
	if err != nil || output.Status != "preview" || output.Job != status {
		t.Fatalf("output=%+v error=%v", output, err)
	}
}

func TestCancelRequiresOneExactSelector(t *testing.T) {
	for _, tc := range []struct {
		input jobactions.CancelInput
		valid bool
	}{
		{jobactions.CancelInput{ID: "job-1"}, true},
		{jobactions.CancelInput{OperationID: "run-1"}, true},
		{jobactions.CancelInput{}, false},
		{jobactions.CancelInput{ID: "job-1", OperationID: "run-1"}, false},
		{jobactions.CancelInput{OperationID: " run-1"}, false},
		{jobactions.CancelInput{OperationID: "run-1", Preview: true}, false},
	} {
		if got := jobactions.ValidateCancelInput(tc.input) == nil; got != tc.valid {
			t.Errorf("input=%+v valid=%v", tc.input, got)
		}
	}
}
