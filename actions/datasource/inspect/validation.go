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
	input.Selector.ProjectLUID = identity.LUID(strings.TrimSpace(string(input.Selector.ProjectLUID)))
	if input.Selector.LUID == "" && (input.Selector.Name == "" || (input.Selector.ProjectPath == "" && input.Selector.ProjectLUID == "")) {
		message := "datasource selection requires a LUID or exact name and project path or project LUID"
		return input, &errs.Error{ID: "datasource.inspect.usage", Kind: errs.KindUsage, Operation: "datasource.inspect", Summary: "Datasource selection requires a LUID or exact name and project path or project LUID.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: message}}}
	}
	if input.Selector.ProjectPath != "" && input.Selector.ProjectLUID != "" {
		return input, &errs.Error{ID: "datasource.inspect.usage", Kind: errs.KindUsage, Operation: "datasource.inspect", Summary: "An exact name cannot be combined with both project path and project LUID.", Retryable: errs.Bool(false), CorrectiveAction: "Provide exactly one of --project or --project-id.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "conflict", Message: "provide exactly one project path or project LUID"}}}
	}
	if input.Selector.LUID != "" && (input.Selector.Name != "" || input.Selector.ProjectPath != "" || input.Selector.ProjectLUID != "") {
		message := "a LUID is authoritative and cannot be combined with name or project selectors"
		return input, &errs.Error{ID: "datasource.inspect.usage", Kind: errs.KindUsage, Operation: "datasource.inspect", Summary: "A LUID is authoritative and cannot be combined with name or project selectors.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "conflict", Message: message}}}
	}
	return input, nil
}
