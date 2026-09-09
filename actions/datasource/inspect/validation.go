package inspect

import (
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
)

// ValidateInput checks an exact selector before authentication.
func ValidateInput(input Input) error { _, err := normalizeInput(input); return err }

func normalizeInput(input Input) (Input, error) {
	input.Selector.LUID = identity.LUID(strings.TrimSpace(string(input.Selector.LUID)))
	input.Selector.Name = strings.TrimSpace(input.Selector.Name)
	input.Selector.ProjectPath = strings.TrimSpace(input.Selector.ProjectPath)
	if input.Selector.LUID == "" && (input.Selector.Name == "" || input.Selector.ProjectPath == "") {
		message := "datasource selection requires a LUID or exact name and project path"
		return input, &errs.Error{ID: "datasource.inspect.usage", Kind: errs.KindUsage, Operation: "datasource.inspect", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: message}}}
	}
	if input.Selector.LUID != "" && (input.Selector.Name != "" || input.Selector.ProjectPath != "") {
		message := "a LUID is authoritative and cannot be combined with name or project selectors"
		return input, &errs.Error{ID: "datasource.inspect.usage", Kind: errs.KindUsage, Operation: "datasource.inspect", Summary: "A LUID is authoritative and cannot be combined with name or project selectors.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "conflict", Message: message}}}
	}
	return input, nil
}
