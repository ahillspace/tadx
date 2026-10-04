package errs

import "errors"

// PublicationMonitorError preserves accepted identity when monitoring is incomplete.
func PublicationMonitorError(operation, environment, site, id, status string, cause error) error {
	if cause == nil {
		return nil
	}
	if known, ok := errors.AsType[*Error](cause); ok && known.Phase == PhasePersistence {
		return cause
	}
	state := OutcomeUnknown
	summary := "Publication was accepted, but local monitoring did not establish its final outcome."
	if status == "succeeded" {
		state, summary = OutcomeConfirmed, "Publication succeeded, but destination confirmation is incomplete."
	}
	if status == "failed" || status == "cancelled" {
		summary = "Tableau reports that publication " + status + "."
	}
	return &Error{ID: operation + ".monitor", Kind: KindOperation, Operation: operation, Environment: environment, Site: site, Summary: summary, Cause: cause, Phase: PhaseVerification, Outcome: state, TableauJobID: id, Retryable: Bool(false), CorrectiveAction: "Recover status using the saved job identity. Do not repeat publication to recover its outcome."}
}
