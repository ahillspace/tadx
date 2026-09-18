package delete

import (
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pulsecontract"
)

// ValidateInput checks local selectors without resolving a site or contacting Tableau.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.LUID) == "" {
		return failure("usage", errs.KindUsage, input, "Pulse metric delete requires an exact LUID.", nil)
	}
	if err := pulsecontract.ValidateLUIDShape("metric", input.LUID); err != nil {
		return failure("usage", errs.KindUsage, input, "Pulse metric delete requires a well-formed exact LUID.", err)
	}
	return nil
}
