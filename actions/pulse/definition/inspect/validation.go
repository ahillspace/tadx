package inspect

import (
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// ValidateInput checks local selectors without resolving a site or contacting Tableau.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.LUID) == "" {
		return definitionError("pulse.definition.inspect.usage", errs.KindUsage, input, "Pulse definition inspect requires an exact LUID.", nil)
	}
	return nil
}
