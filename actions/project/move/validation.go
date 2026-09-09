package move

import "strings"

// ValidateInput checks caller-controlled arguments before local or remote setup.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" {
		return usage("environment", "project move requires an explicit environment")
	}
	if in.ProjectSelector.LUID == "" && strings.TrimSpace(in.ProjectSelector.ProjectPath) == "" {
		return usage("selector", "project move requires a project LUID or exact path")
	}
	if in.ProjectSelector.LUID != "" && strings.TrimSpace(in.ProjectSelector.ProjectPath) != "" {
		return usage("selector", "a project LUID cannot be combined with a project path")
	}
	hasParent := in.ParentSelector.LUID != "" || strings.TrimSpace(in.ParentSelector.ProjectPath) != ""
	if hasParent == in.TopLevel {
		return usage("parent", "use exactly one parent project selector or --top-level")
	}
	if in.ParentSelector.LUID != "" && strings.TrimSpace(in.ParentSelector.ProjectPath) != "" {
		return usage("parent", "a parent project LUID cannot be combined with a project path")
	}
	return nil
}
