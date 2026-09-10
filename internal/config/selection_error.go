package config

import (
	"fmt"
	"sort"
	"strings"
)

// selectionError preserves local environment recovery without coupling config
// resolution to a CLI action or issuing an authoritative remote lookup.
type selectionError struct {
	alias     string
	available []string
}

func (e *selectionError) Error() string {
	if e.alias == "" {
		return "no environment selected and no default environment is configured"
	}
	return fmt.Sprintf("environment %q does not exist; select a configured environment alias, not a Tableau site name or URL", e.alias)
}
func (e *selectionError) Retryable() bool { return false }
func (e *selectionError) CorrectiveAction() string {
	advice := "Use --env <alias> to select a configured environment alias. Run tadx env list to inspect configured targets."
	if len(e.available) > 0 {
		advice += " Available aliases: " + strings.Join(e.available, ", ") + "."
	}
	return advice
}
func (e *selectionError) PrerequisiteKind() string     { return "environment" }
func (e *selectionError) PrerequisiteResource() string { return e.alias }
func (e *selectionError) PrerequisiteSummary() string {
	return "Select a configured environment alias."
}
func (c Config) selectionError(alias string) error {
	names := make([]string, 0, len(c.Environments))
	for name := range c.Environments {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 12 {
		names = append(names[:12], "more available through env list")
	}
	return &selectionError{alias: alias, available: names}
}
