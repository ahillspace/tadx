package config

import (
	"errors"
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// LoadError retains the selected settings file and its precise parsing or validation cause.
type LoadError struct {
	Path  string
	Cause error
}

var yamlScalarExcerpt = regexp.MustCompile("`[^`]*`")

func (e *LoadError) Error() string {
	cause := e.Cause.Error()
	if _, ok := errors.AsType[*yaml.TypeError](e.Cause); ok {
		// YAML conversion errors can quote pasted credential values. Keep the
		// line, expected type, field, and workspace-routing context, not scalars.
		cause = yamlScalarExcerpt.ReplaceAllString(cause, "[value redacted]")
	}
	return fmt.Sprintf("CLI settings %q: %s", e.Path, cause)
}
func (e *LoadError) Unwrap() error { return e.Cause }
func (*LoadError) Retryable() bool { return false }
func (*LoadError) CorrectiveAction() string {
	return "Correct the reported field in the selected CLI settings file; use --workspace for a registered workspace, not its manifest as --config."
}
func (*LoadError) PrerequisiteKind() string       { return "configuration" }
func (e *LoadError) PrerequisiteResource() string { return e.Path }
func (*LoadError) PrerequisiteSummary() string {
	return "Valid CLI settings are required before dependent operations can run."
}
