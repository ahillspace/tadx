package update

import "strings"

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
	if in.OwnerLUID == nil || strings.TrimSpace(*in.OwnerLUID) == "" {
		return usage("owner_id", "flow update requires an exact owner LUID")
	}
	return nil
}
