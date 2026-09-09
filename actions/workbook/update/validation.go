package update

import "strings"

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
	if input.Name == nil && input.OwnerLUID == nil {
		return usage("changes", "workbook update requires a name or owner LUID")
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return usage("name", "workbook update name cannot be empty")
	}
	if input.OwnerLUID != nil && strings.TrimSpace(*input.OwnerLUID) == "" {
		return usage("owner_id", "workbook update owner LUID cannot be empty")
	}
	return nil
}
