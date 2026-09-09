package move

import (
	"strings"
)

// ValidateInput checks caller-controlled arguments before dependency setup.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.Environment) == "" {
		return usage("environment", "flow move requires an explicit environment")
	}
	selector := input.FlowSelector
	if selector.LUID == "" && (strings.TrimSpace(selector.Name) == "" || strings.TrimSpace(selector.ProjectPath) == "") {
		return usage("selector", "flow move requires a LUID or exact name and project path")
	}
	if selector.LUID != "" && (selector.Name != "" || selector.ProjectPath != "") {
		return usage("selector", "a LUID cannot be combined with name or project selectors")
	}
	project := input.ProjectSelector
	if (project.LUID == "" && strings.TrimSpace(project.ProjectPath) == "") || (project.LUID != "" && project.ProjectPath != "") {
		return usage("project", "flow move requires one exact destination project selector")
	}
	return nil
}
