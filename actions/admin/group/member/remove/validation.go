package remove

import "strings"

// ValidateInput checks local selectors without contacting Tableau.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.GroupLUID) == "" || strings.TrimSpace(in.UserLUID) == "" {
		return usage("--environment, --group-id, and --user-id are required")
	}

	return nil
}
