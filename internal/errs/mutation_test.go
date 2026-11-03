package errs_test

import (
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

type mutationResponse struct{ status int }

func (e mutationResponse) Error() string          { return "mutation response" }
func (e mutationResponse) HTTPStatus() int        { return e.status }
func (mutationResponse) Retryable() bool          { return true }
func (mutationResponse) CorrectiveAction() string { return "Repeat the request." }

type unattemptedMutation struct{}

func (unattemptedMutation) Error() string      { return "not submitted" }
func (unattemptedMutation) NotAttempted() bool { return true }

func TestMutationFailureUsesOperationEvidenceAndRecovery(t *testing.T) {
	for _, test := range []struct {
		name, status string
		cause        error
		phase        errs.Phase
		outcome      errs.Outcome
	}{
		{"uncertain service response", "unknown", mutationResponse{503}, errs.PhaseSubmission, errs.OutcomeUnknown},
		{"uncertain malformed response", "unknown", mutationResponse{200}, errs.PhaseSubmission, errs.OutcomeUnknown},
		{"uncertain receipt dominates response", "unknown", mutationResponse{403}, errs.PhaseSubmission, errs.OutcomeUnknown},
		{"definite rejection", "", mutationResponse{403}, errs.PhaseSubmission, errs.OutcomeNotApplied},
		{"acknowledged verification failure", "partial", errors.New("description differs"), errs.PhaseVerification, errs.OutcomeConfirmed},
		{"admission denied", "", unattemptedMutation{}, errs.PhaseSetup, errs.OutcomeNotAttempted},
		{"explicit persistence failure", "", &errs.Error{Phase: errs.PhasePersistence, Outcome: errs.OutcomeConfirmed}, errs.PhasePersistence, errs.OutcomeConfirmed},
		{"unclassified mutation error", "", errors.New("response unavailable"), errs.PhaseSubmission, errs.OutcomeUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := errs.MutationFailure(errs.Error{ID: "content.mutation.failed", Cause: test.cause, CorrectiveAction: "Inspect the exact target and request."}, test.status)
			if failure.Phase != test.phase || failure.Outcome != test.outcome || failure.Retryable == nil || *failure.Retryable || failure.CorrectiveAction != "Inspect the exact target and request." {
				t.Fatalf("failure=%#v", failure)
			}
		})
	}
}
