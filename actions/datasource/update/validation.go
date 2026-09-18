package update

import (
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
)

// ValidateInput checks caller-controlled arguments before local or remote setup.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" {
		return usage("environment", "datasource update requires an explicit environment")
	}
	if in.Selector.LUID == "" && (strings.TrimSpace(in.Selector.Name) == "" || strings.TrimSpace(in.Selector.ProjectPath) == "") {
		return usage("selector", "datasource update requires a LUID or exact name and project path")
	}
	if in.Selector.LUID != "" && (strings.TrimSpace(in.Selector.Name) != "" || strings.TrimSpace(in.Selector.ProjectPath) != "") {
		return usage("selector", "a datasource LUID cannot be combined with name or project path")
	}
	if in.Name == nil && in.OwnerLUID == nil {
		return usage("changes", "datasource update requires a name or owner LUID")
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return usage("name", "datasource update name cannot be empty")
	}
	if in.OwnerLUID != nil {
		if err := identity.ValidateLUIDShape("owner", *in.OwnerLUID); err != nil {
			return usage("owner_id", err.Error())
		}
	}
	return nil
}
