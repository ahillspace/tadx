package refresh

import "github.com/ahillspace/tadx/internal/errs"

// ValidateInput checks scopes without requiring resolved credentials or a site.
func ValidateInput(input Input) error {
	_, _, err := normalizeScopes(input.Scopes)
	if err != nil {
		return failure("catalog.refresh.usage", errs.KindUsage, input, err.Error(), err)
	}
	return nil
}
