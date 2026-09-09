package delete

// ValidateInput checks local permission selectors before authentication.
func ValidateInput(input Input) error { return Validate(input) }
