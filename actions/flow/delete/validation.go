package delete

import (
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// ValidateInput checks caller-controlled arguments before dependency setup.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.Environment) == "" {
		return errs.New(errs.KindUsage, "flow delete requires an explicit environment")
	}
	selector := input.Selector
	if strings.TrimSpace(string(selector.LUID)) == "" && (strings.TrimSpace(selector.Name) == "" || strings.TrimSpace(selector.ProjectPath) == "") {
		return errs.New(errs.KindUsage, "flow selection requires a LUID or exact name and project path")
	}
	if selector.LUID != "" && (selector.Name != "" || selector.ProjectPath != "") {
		return errs.New(errs.KindUsage, "a LUID cannot be combined with name or project selectors")
	}
	return nil
}
