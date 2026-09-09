package inspect

import (
	"github.com/ahillspace/tadx/internal/errs"
)

// ValidateInput checks an exact selector before authentication.
func ValidateInput(input Input) error { _, err := normalizeInput(input); return err }

func normalizeInput(input Input) (Input, error) {
	if input.Selector.LUID == "" && (input.Selector.Name == "" || input.Selector.ProjectPath == "") {
		return input, &errs.Error{ID: "flow.inspect.usage", Kind: errs.KindUsage, Operation: "flow.inspect", Summary: "flow selection requires a LUID or exact name and project path", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "flow selection requires a LUID or exact name and project path"}}}
	}
	return input, nil
}
