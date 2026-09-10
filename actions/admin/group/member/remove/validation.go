package remove

import "strings"

// ValidateInput checks local selectors without contacting Tableau.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.GroupLUID) == "" {
		return usage("--environment and --group-id are required")
	}
	if (in.UserLUID == "") == (in.Username == "") {
		return usage("provide exactly one of --user-id or --username")
	}
	if strings.TrimSpace(in.UserLUID) != in.UserLUID || strings.TrimSpace(in.Username) != in.Username {
		return usage("user selectors must not contain surrounding whitespace")
	}
	return nil
}
