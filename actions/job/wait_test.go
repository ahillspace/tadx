package job_test

import (
	"context"
	"errors"
	"testing"

	jobactions "github.com/ahillspace/tadx/actions/job"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type waitSource struct {
	result jobactions.WaitResult
	err    error
}

func (s waitSource) Wait(context.Context, jobactions.WaitInput) (jobactions.WaitResult, error) {
	return s.result, s.err
}

func TestExecutePreservesAcceptedIdentityWhenRecoveryFails(t *testing.T) {
	status := value.JobStatus{ID: "job-1", Status: "pending"}
	accepted := jobactions.WaitResult{Status: status, Environment: "dev", Site: "site", ReceiptPath: "jobs/job-1.json"}
	failure := &errs.Error{ID: "job.wait.recovery", Outcome: errs.OutcomeUnknown}
	output, err := jobactions.Wait(t.Context(), (waitSource{result: accepted, err: failure}).Wait, jobactions.WaitInput{Receipt: "jobs/job-1.json"})
	if !errors.Is(err, failure) || output.Job != status || output.ReceiptPath != accepted.ReceiptPath || len(output.Help) != 2 {
		t.Fatalf("output=%+v error=%v", output, err)
	}
}
