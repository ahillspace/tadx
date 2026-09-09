package inspect

import (
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// ValidateInput checks one exact selector before authentication.
func ValidateInput(input Input) error {
	id, name := strings.TrimSpace(input.Selector.LUID), strings.TrimSpace(input.Selector.NameOrEmail)
	if (id == "") == (name == "") {
		return &errs.Error{ID: "admin.user.inspect.usage", Kind: errs.KindUsage, Operation: "admin.user.inspect", Summary: "Provide exactly one authoritative LUID or exact user name.", Retryable: errs.Bool(false), CorrectiveAction: "Use --id or --name, but not both.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "invalid", Message: "exactly one selector is required"}}}
	}
	return nil
}
