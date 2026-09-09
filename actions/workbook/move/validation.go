package move

import "strings"

// ValidateInput checks caller-controlled arguments before local or remote setup.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.Environment) == "" {
		return usage("environment", "workbook move requires an explicit environment")
	}
	if input.WorkbookSelector.LUID == "" && (strings.TrimSpace(input.WorkbookSelector.Name) == "" || strings.TrimSpace(input.WorkbookSelector.ProjectPath) == "") {
		return usage("selector", "workbook move requires a LUID or exact name and project path")
	}
	if input.WorkbookSelector.LUID != "" && (strings.TrimSpace(input.WorkbookSelector.Name) != "" || strings.TrimSpace(input.WorkbookSelector.ProjectPath) != "") {
		return usage("selector", "a workbook LUID cannot be combined with name or project path")
	}
	if input.ProjectSelector.LUID == "" && strings.TrimSpace(input.ProjectSelector.ProjectPath) == "" {
		return usage("project", "workbook move requires a destination project LUID or exact path")
	}
	if input.ProjectSelector.LUID != "" && strings.TrimSpace(input.ProjectSelector.ProjectPath) != "" {
		return usage("project", "a project LUID cannot be combined with a project path")
	}
	return nil
}
