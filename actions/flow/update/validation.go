package update

import (
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
)

// ValidateInput checks caller-controlled arguments before local or remote setup.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" {
		return usage("environment", "flow update requires an explicit environment")
	}
	if in.Selector.LUID == "" && (strings.TrimSpace(in.Selector.Name) == "" || strings.TrimSpace(in.Selector.ProjectPath) == "") {
		return usage("selector", "flow update requires a LUID or exact name and project path")
	}
	if in.Selector.LUID != "" && (strings.TrimSpace(in.Selector.Name) != "" || strings.TrimSpace(in.Selector.ProjectPath) != "") {
		return usage("selector", "a flow LUID cannot be combined with name or project path")
	}
	if in.OwnerLUID == nil {
		return usage("owner_id", "flow update requires an exact owner LUID")
	}
	if err := identity.ValidateLUIDShape("owner", *in.OwnerLUID); err != nil {
		return usage("owner_id", err.Error())
	}
	return nil
}
