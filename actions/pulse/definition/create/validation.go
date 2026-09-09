package create

import "github.com/ahillspace/tadx/internal/errs"

// ValidateInput checks the complete authoring intent without live field validation.
func ValidateInput(input Input) error {
	if _, err := requestFromIntent(input.Intent); err != nil {
		return createError("pulse.definition.create.usage", errs.KindUsage, input, "Pulse definition intent is invalid.", err)
	}
	return nil
}
