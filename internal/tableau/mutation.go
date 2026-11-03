package tableau

import "errors"

// MutationUncertain reports an attempted write without a definitive rejection.
// A server or intermediary failure does not prove that Tableau applied nothing.
func MutationUncertain(err error) bool {
	if !SubmissionAttempted(err) {
		return false
	}
	if rejected, ok := errors.AsType[*UpstreamError](err); ok && rejected.StatusCode >= 400 && rejected.StatusCode < 500 {
		return false
	}
	return true
}
