package fork

import (
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// ValidateInput validates local changes; allowed fields and grains require live evidence.
func ValidateInput(input Input) error {
	timeframe := strings.ToUpper(strings.TrimSpace(input.Timeframe))
	if strings.TrimSpace(input.MetricLUID) == "" {
		return fail("pulse.metric.fork.usage", errs.KindUsage, input, "Pulse metric fork requires an exact source metric LUID.", nil)
	}
	if timeframe == "" && len(input.Filters) == 0 {
		return fail("pulse.metric.fork.usage", errs.KindUsage, input, "Pulse metric fork requires a timeframe or dimensional filter.", nil)
	}
	if timeframe != "" {
		if _, ok := measurementPeriod(timeframe, input.CustomDays); !ok {
			return fail("pulse.metric.fork.usage", errs.KindUsage, input, "Pulse metric fork timeframe is not supported.", nil)
		}
	}
	if input.CustomDays != 0 && timeframe != "CUSTOM_N_DAYS" {
		return fail("pulse.metric.fork.usage", errs.KindUsage, input, "--days requires CUSTOM_N_DAYS.", nil)
	}
	for _, filter := range canonicalFilters(input.Filters) {
		if filter.Field == "" || len(filter.Values) == 0 || !validFilterValues(filter.Values) {
			return fail("pulse.metric.fork.usage", errs.KindUsage, input, "Every dimensional filter must name a field and bounded nonempty values.", nil)
		}
	}
	return nil
}
