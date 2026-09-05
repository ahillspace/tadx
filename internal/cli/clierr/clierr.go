// Package clierr builds the shared structured usage error used by every CLI command.
package clierr

import (
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

// Usage builds the canonical structured usage error for one CLI operation.
func Usage(operation string, cause error) error {
	return &errs.Error{Kind: errs.KindUsage, Operation: operation, Summary: cause.Error(), Cause: cause}
}

type renderedError struct{ cause error }

func (e renderedError) Error() string { return e.cause.Error() }
func (e renderedError) Unwrap() error { return e.cause }

// Rendered marks an error whose complete diagnostic document is already written.
func Rendered(cause error) error { return renderedError{cause: cause} }

// IsRendered reports whether a command already wrote the complete diagnostic document.
func IsRendered(err error) bool {
	var rendered renderedError
	return errors.As(err, &rendered)
}
