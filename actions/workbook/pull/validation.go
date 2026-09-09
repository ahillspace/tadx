package pull

import (
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
)

// ValidateInput checks caller-controlled arguments before dependency setup.
func ValidateInput(input Input) error {
	selector := input.Selector
	if selector.LUID == "" && selector.Name == "" && selector.ProjectPath == "" {
		selector = identity.Selector{LUID: identity.LUID(input.LUID), Name: input.Name, ProjectPath: input.ProjectPath}
	}
	if strings.TrimSpace(string(selector.LUID)) == "" && strings.TrimSpace(selector.Name) == "" {
		return errs.New(errs.KindUsage, "one of workbook LUID or name is required")
	}
	if input.Selector.LUID != "" && (input.Selector.Name != "" || input.Selector.ProjectPath != "") {
		return errs.New(errs.KindUsage, "a LUID cannot be combined with name or project selectors")
	}
	return nil
}
