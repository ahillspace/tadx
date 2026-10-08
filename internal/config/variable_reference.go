package config

import (
	"regexp"
)

const maxVariableReferenceLength = 128

var variableReferencePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// VariableReferenceRule describes an accepted PAT environment-variable
// reference. Messages use it instead of the rejected value, because a value
// that is not a variable name is most likely a pasted secret.
const VariableReferenceRule = "must be an environment variable name of letters, digits, and underscores that does not start with a digit and is at most 128 characters"

// ValidVariableReference reports whether reference is a portable
// environment-variable name. Callers must not echo a rejected reference.
func ValidVariableReference(reference string) bool {
	return len(reference) <= maxVariableReferenceLength && variableReferencePattern.MatchString(reference)
}
