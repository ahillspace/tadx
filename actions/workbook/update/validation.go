package update

import (
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
)

// ValidateInput checks caller-controlled arguments before local or remote setup.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.Environment) == "" {
		return usage("environment", "workbook update requires an explicit environment")
	}
	if input.Selector.LUID == "" && (strings.TrimSpace(input.Selector.Name) == "" || strings.TrimSpace(input.Selector.ProjectPath) == "") {
		return usage("selector", "workbook update requires a LUID or exact name and project path")
	}
	if input.Selector.LUID != "" && (strings.TrimSpace(input.Selector.Name) != "" || strings.TrimSpace(input.Selector.ProjectPath) != "") {
		return usage("selector", "a workbook LUID cannot be combined with name or project path")
	}
	if input.Name == nil && input.OwnerLUID == nil && input.Description == nil {
		return usage("changes", "workbook update requires a name, owner LUID, or description")
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return usage("name", "workbook update name cannot be empty")
	}
	if input.OwnerLUID != nil {
		if err := identity.ValidateLUIDShape("owner", *input.OwnerLUID); err != nil {
			return usage("owner_id", err.Error())
		}
	}
	return nil
}
