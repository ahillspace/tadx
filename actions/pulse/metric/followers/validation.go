package followers

import (
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pulsecontract"
)

// ValidateInput checks local selectors without resolving a site or contacting Tableau.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.MetricLUID) == "" {
		return fail("pulse.metric.followers.usage", errs.KindUsage, input, "Pulse metric followers requires an exact LUID.", nil)
	}
	if err := pulsecontract.ValidateLUIDShape("metric", input.MetricLUID); err != nil {
		return fail("pulse.metric.followers.usage", errs.KindUsage, input, "Pulse metric followers requires a well-formed exact metric LUID.", err)
	}
	return nil
}
