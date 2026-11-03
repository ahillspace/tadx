package errs

import "errors"

// MutationFailure records the operation's evidence independently of transport
// retryability. The caller owns the recovery instruction and result receipt.
func MutationFailure(failure Error, status string) *Error {
	failure.Retryable = new(false)
	if failure.TableauRequestID == "" {
		failure.TableauRequestID = TableauRequestID(failure.Cause)
	}
	failure.Phase, failure.Outcome = PhaseSubmission, OutcomeUnknown
	switch status {
	case "succeeded", "partial":
		failure.Phase, failure.Outcome = PhaseVerification, OutcomeConfirmed
	case "not_attempted":
		failure.Phase, failure.Outcome = PhaseSetup, OutcomeNotAttempted
	case "not_applied":
		failure.Outcome = OutcomeNotApplied
	case "unknown":
		// An explicit uncertain receipt takes precedence over an HTTP status.
	default:
		if known, ok := errors.AsType[*Error](failure.Cause); ok && known.Outcome != "" {
			failure.Outcome = known.Outcome
			if known.Phase != "" {
				failure.Phase = known.Phase
			}
		} else if denied, ok := errors.AsType[interface {
			error
			NotAttempted() bool
		}](failure.Cause); ok && denied.NotAttempted() {
			failure.Phase, failure.Outcome = PhaseSetup, OutcomeNotAttempted
		} else if response, ok := errors.AsType[interface {
			error
			HTTPStatus() int
		}](failure.Cause); ok && response.HTTPStatus() >= 400 && response.HTTPStatus() < 500 {
			failure.Outcome = OutcomeNotApplied
		}
	}
	return &failure
}
