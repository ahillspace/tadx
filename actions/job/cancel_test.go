package job

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type cancelSource struct {
	result CancelResult
	err    error
}

func (s cancelSource) Cancel(context.Context, CancelInput) (CancelResult, error) {
	return s.result, s.err
}

func TestExecutePreservesRequestIdentityOnUnknownConfirmation(t *testing.T) {
	status := value.JobStatus{ID: "job-1", Status: "running"}
	accepted := CancelResult{Status: status, Environment: "dev", Site: "site", RequestID: "request-1"}
	failure := &errs.Error{ID: "job.cancel.confirmation", Outcome: errs.OutcomeUnknown}
	output, err := cancelOutput(t.Context(), (cancelSource{result: accepted, err: failure}).Cancel, CancelInput{ID: "job-1"})
	if !errors.Is(err, failure) || output.Job != status || output.RequestID != accepted.RequestID || output.Confirmed || len(output.Help) != 1 {
		t.Fatalf("output=%+v error=%v", output, err)
	}
}

func TestExecutePreviewProjectsInspectedJob(t *testing.T) {
	status := value.JobStatus{ID: "job-1", Status: "running"}
	output, err := cancelOutput(t.Context(), (cancelSource{result: CancelResult{Status: status, Environment: "dev", Site: "site"}}).Cancel, CancelInput{ID: "job-1", Preview: true})
	if err != nil || output.Status != "preview" || output.Job != status {
		t.Fatalf("output=%+v error=%v", output, err)
	}
}
