package create

import "github.com/ahillspace/tadx/internal/errs"

// ValidateInput checks the complete authoring intent without live field validation.
func ValidateInput(input Input) error {
	_, err := validatedRequest(input)
	return err
}

func validatedRequest(input Input) (CreateRequest, error) {
	request, err := requestFromIntent(input.Intent)
	if err != nil {
		return CreateRequest{}, createError("pulse.definition.create.usage", errs.KindUsage, input, "Pulse definition intent is invalid.", err)
	}
	return request, nil
}
