package delete

import (
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.UserLUID) == "" {
		return &errs.Error{ID: "admin.user.delete.usage", Kind: errs.KindUsage, Operation: "admin.user.delete", Summary: "admin user delete requires explicit environment and user LUID", Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact environment and user LUID.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "admin user delete requires explicit environment and user LUID"}}}
	}
	return nil
}
