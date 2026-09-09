package delete

import (
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
)

// ValidateInput checks an exact selector before authentication.
func ValidateInput(input Input) error { _, err := normalizeInput(input); return err }

func normalizeInput(input Input) (Input, error) {
	input.Selector.LUID = identity.LUID(strings.TrimSpace(string(input.Selector.LUID)))
	input.Selector.Name = strings.TrimSpace(input.Selector.Name)
	input.Selector.ProjectPath = strings.TrimSpace(input.Selector.ProjectPath)
	if input.Selector.LUID == "" && (input.Selector.Name == "" || input.Selector.ProjectPath == "") {
		return input, selectorUsage("required", "workbook selection requires a LUID or exact name and project path", "Workbook selection requires a LUID or exact name and project path.")
	}
	if input.Selector.LUID != "" && (input.Selector.Name != "" || input.Selector.ProjectPath != "") {
		return input, selectorUsage("conflict", "a LUID is authoritative and cannot be combined with name or project selectors", "A LUID is authoritative and cannot be combined with name or project selectors.")
	}
	return input, nil
}
