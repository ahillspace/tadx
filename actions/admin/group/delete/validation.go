package delete

import (
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.GroupLUID) == "" {
		return &errs.Error{ID: "admin.group.delete.usage", Kind: errs.KindUsage, Operation: "admin.group.delete", Summary: "admin group delete requires explicit environment and group LUID", Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact environment and group LUID.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "admin group delete requires explicit environment and group LUID"}}}
	}
	return nil
}
