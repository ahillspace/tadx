package create

import "strings"

// ValidateInput checks caller-controlled arguments before local or remote setup.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.Environment) == "" {
		return usage("environment", "project create requires an explicit environment")
	}
	if strings.TrimSpace(input.Name) == "" {
		return usage("name", "project create requires a name")
	}
	if strings.Contains(input.Name, "/") {
		return usage("name", "project create name cannot contain a slash")
	}
	if input.ParentSelector.Name != "" || (input.ParentSelector.LUID != "" && strings.TrimSpace(input.ParentSelector.ProjectPath) != "") {
		return usage("parent", "use either a parent LUID or an exact parent project path")
	}
	if !validContentPermissions(input.ContentPermissions) {
		return usage("content_permissions", "project create content permissions are invalid")
	}
	return nil
}
