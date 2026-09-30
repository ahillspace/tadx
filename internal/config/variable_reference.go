package config

import (
	"fmt"
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

// variableReferenceViolations reports explicit PAT references that are not
// variable names, naming only the environment and the field.
func variableReferenceViolations(alias string, auth Auth) []string {
	var violations []string
	for _, reference := range []struct{ field, value string }{
		{field: "pat_name_env", value: auth.PATNameEnv},
		{field: "pat_secret_env", value: auth.PATSecretEnv},
	} {
		if reference.value != "" && !ValidVariableReference(reference.value) {
			violations = append(violations, fmt.Sprintf("environment %q %s %s; the value is not shown because it may be a secret, and a PAT saved there should be revoked", alias, reference.field, VariableReferenceRule))
		}
	}
	return violations
}
