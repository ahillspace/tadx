package update

import "strings"

// ValidateInput checks caller-controlled arguments before local or remote setup.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.Environment) == "" {
		return usage("environment", "project update requires an explicit environment")
	}
	if input.Selector.LUID == "" && strings.TrimSpace(input.Selector.ProjectPath) == "" {
		return usage("selector", "project update requires a project LUID or exact project path")
	}
	if input.Selector.Name != "" || (input.Selector.LUID != "" && strings.TrimSpace(input.Selector.ProjectPath) != "") {
		return usage("selector", "use either a project LUID or an exact project path")
	}
	if input.Name == nil && input.Description == nil && input.ContentPermissions == nil {
		return usage("changes", "project update requires at least one explicit metadata change")
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return usage("name", "project update name cannot be empty")
	}
	if input.Name != nil && strings.Contains(*input.Name, "/") {
		return usage("name", "project update name cannot contain a slash")
	}
	if input.ContentPermissions != nil && !validContentPermissions(*input.ContentPermissions) {
		return usage("content_permissions", "project update content permissions are invalid")
	}
	return nil
}
