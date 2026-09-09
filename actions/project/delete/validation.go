package delete

import (
	"strings"
)

// ValidateInput checks caller-controlled arguments before dependency setup.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.Environment) == "" {
		return usage("environment", "project delete requires an explicit environment")
	}
	if strings.TrimSpace(input.ProjectLUID) == "" {
		return usage("project_id", "project delete requires an authoritative project LUID")
	}
	return nil
}
