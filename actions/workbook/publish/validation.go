package publish

import (
	"strings"
)

// ValidateInput checks caller-controlled arguments before dependency setup.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.ArtifactPath) == "" && strings.TrimSpace(input.File) == "" && strings.TrimSpace(input.ArtifactID) == "" && strings.TrimSpace(input.ArtifactName) == "" {
		return usage("artifact", "an explicit workbook artifact is required")
	}
	if input.ProjectSelector.LUID != "" && input.ProjectSelector.ProjectPath != "" && !input.SourceDefaulted {
		return usage("project", "a project LUID cannot be combined with a project path")
	}
	return nil
}
