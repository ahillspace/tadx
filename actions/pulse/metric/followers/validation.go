package followers

import (
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// ValidateInput checks local selectors without resolving a site or contacting Tableau.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.MetricLUID) == "" {
		return fail("pulse.metric.followers.usage", errs.KindUsage, input, "Pulse metric followers requires an exact LUID.", nil)
	}
	return nil
}
