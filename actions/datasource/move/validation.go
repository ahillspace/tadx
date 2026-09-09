package move

import "strings"

// ValidateInput checks caller-controlled arguments before local or remote setup.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" {
		return usage("environment", "datasource move requires an explicit environment")
	}
	if in.DatasourceSelector.LUID == "" && (strings.TrimSpace(in.DatasourceSelector.Name) == "" || strings.TrimSpace(in.DatasourceSelector.ProjectPath) == "") {
		return usage("selector", "datasource move requires a LUID or exact name and project path")
	}
	if in.DatasourceSelector.LUID != "" && (strings.TrimSpace(in.DatasourceSelector.Name) != "" || strings.TrimSpace(in.DatasourceSelector.ProjectPath) != "") {
		return usage("selector", "a datasource LUID cannot be combined with name or project path")
	}
	if in.ProjectSelector.LUID == "" && strings.TrimSpace(in.ProjectSelector.ProjectPath) == "" {
		return usage("project", "datasource move requires a destination project LUID or exact path")
	}
	if in.ProjectSelector.LUID != "" && strings.TrimSpace(in.ProjectSelector.ProjectPath) != "" {
		return usage("project", "a project LUID cannot be combined with a project path")
	}
	return nil
}
