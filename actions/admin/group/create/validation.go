package create

import (
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.Name) == "" {
		return &errs.Error{ID: "admin.group.create.usage", Kind: errs.KindUsage, Operation: "admin.group.create", Summary: "admin group create requires explicit environment and name", Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact environment and group name.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "admin group create requires explicit environment and name"}}}
	}
	return nil
}
