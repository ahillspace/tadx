package tableau

import (
	"errors"
	"testing"
)

func TestMutationUncertainRequiresSubmissionAndDefinitiveRejectionEvidence(t *testing.T) {
	for _, test := range []struct {
		name      string
		err       error
		uncertain bool
	}{
		{"not submitted", errors.New("invalid destination"), false},
		{"request transport failure", &requestError{cause: errors.New("connection closed")}, true},
		{"unavailable service", &UpstreamError{StatusCode: 503}, true},
		{"upstream server failure", &UpstreamError{StatusCode: 500}, true},
		{"definite rejection", &UpstreamError{StatusCode: 403}, false},
		{"invalid successful response", NewProtocolError("mutation", Response{StatusCode: 200}, errors.New("invalid receipt"), false), true},
		{"incomplete rejection body", &responseReadError{statusCode: 403, cause: errors.New("body truncated")}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if actual := MutationUncertain(test.err); actual != test.uncertain {
				t.Fatalf("MutationUncertain(%v)=%t want%t", test.err, actual, test.uncertain)
			}
		})
	}
}
