// Package clierr builds the shared structured usage error used by every CLI command.
package clierr

import "github.com/ahillspace/tadx/internal/errs"

// Usage builds the canonical structured usage error for one CLI operation.
func Usage(operation string, cause error) error {
	return &errs.Error{Kind: errs.KindUsage, Operation: operation, Summary: cause.Error(), Cause: cause}
}
