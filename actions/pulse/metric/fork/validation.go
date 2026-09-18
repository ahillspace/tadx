package fork

import (
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pulsecontract"
)

// ValidateInput validates local changes; allowed fields and grains require live evidence.
func ValidateInput(input Input) error {
	timeframe := strings.ToUpper(strings.TrimSpace(input.Timeframe))
	if strings.TrimSpace(input.MetricLUID) == "" {
		return fail("pulse.metric.fork.usage", errs.KindUsage, input, "Pulse metric fork requires an exact source metric LUID.", nil)
	}
	if err := pulsecontract.ValidateLUIDShape("metric", input.MetricLUID); err != nil {
		return fail("pulse.metric.fork.usage", errs.KindUsage, input, "Pulse metric fork requires a well-formed exact source metric LUID.", err)
	}
	if timeframe == "" && len(input.Filters) == 0 {
		return fail("pulse.metric.fork.usage", errs.KindUsage, input, "Pulse metric fork requires a timeframe or dimensional filter.", nil)
	}
	daysSet := input.CustomDaysSet || input.CustomDays != 0
	if timeframe == "CUSTOM_N_DAYS" && !daysSet {
		return fail("pulse.metric.fork.usage", errs.KindUsage, input, "--period CUSTOM_N_DAYS requires --days.", nil)
	}
	if daysSet && timeframe != "CUSTOM_N_DAYS" {
		return fail("pulse.metric.fork.usage", errs.KindUsage, input, "--days requires CUSTOM_N_DAYS.", nil)
	}
	if daysSet && !IsSupportedCustomDays(input.CustomDays) {
		return fail("pulse.metric.fork.usage", errs.KindUsage, input, "--days must be one of 7, 14, 30, 60, or 90.", nil)
	}
	if timeframe != "" {
		if _, ok := measurementPeriod(timeframe, input.CustomDays); !ok {
			return fail("pulse.metric.fork.usage", errs.KindUsage, input, "Pulse metric fork timeframe is not supported.", nil)
		}
	}
	for _, filter := range canonicalFilters(input.Filters) {
		if filter.Field == "" || len(filter.Values) == 0 || !validFilterValues(filter.Values) {
			return fail("pulse.metric.fork.usage", errs.KindUsage, input, "Every dimensional filter must name a field and bounded nonempty values.", nil)
		}
	}
	return nil
}

// IsSupportedCustomDays reports whether days is one of Tableau Pulse's bounded
// custom trailing periods.
func IsSupportedCustomDays(days int) bool {
	return days == 7 || days == 14 || days == 30 || days == 60 || days == 90
}
