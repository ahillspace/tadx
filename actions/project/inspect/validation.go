package inspect

import (
	"github.com/ahillspace/tadx/internal/errs"
)

// ValidateInput checks an exact selector before authentication.
func ValidateInput(input Input) error { _, err := normalizeInput(input); return err }

func normalizeInput(input Input) (Input, error) {
	if input.Selector.LUID == "" && input.Selector.ProjectPath == "" {
		return input, &errs.Error{ID: "project.inspect.usage", Kind: errs.KindUsage, Operation: "project.inspect", Summary: "project LUID or exact project path is required", Retryable: errs.Bool(false), CorrectiveAction: "Provide a project LUID or an exact project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "project LUID or exact project path is required"}}}
	}
	return input, nil
}
