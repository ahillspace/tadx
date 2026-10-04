package job

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type waitSource struct {
	result WaitResult
	err    error
}

func (s waitSource) Wait(context.Context, WaitInput) (WaitResult, error) {
	return s.result, s.err
}

func TestExecutePreservesAcceptedIdentityWhenRecoveryFails(t *testing.T) {
	status := value.JobStatus{ID: "job-1", Status: "pending"}
	accepted := WaitResult{Status: status, Environment: "dev", Site: "site", ReceiptPath: "jobs/job-1.json"}
	failure := &errs.Error{ID: "job.wait.recovery", Outcome: errs.OutcomeUnknown}
	output, err := waitOutput(t.Context(), (waitSource{result: accepted, err: failure}).Wait, WaitInput{Receipt: "jobs/job-1.json"})
	if !errors.Is(err, failure) || output.Job != status || output.ReceiptPath != accepted.ReceiptPath || len(output.Help) != 2 {
		t.Fatalf("output=%+v error=%v", output, err)
	}
}
