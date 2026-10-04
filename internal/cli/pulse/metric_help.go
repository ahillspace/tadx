package pulse

import (
	"strconv"

	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

func metricHelpFacts(command *cobra.Command, action string) {
	if action == "fork" {
		helpmeta.Choices(command, "period", "TODAY", "THIS_WEEK", "MONTH_TO_DATE", "QUARTER_TO_DATE", "YEAR_TO_DATE", "YESTERDAY", "LAST_WEEK", "LAST_MONTH", "LAST_QUARTER", "LAST_YEAR", "LAST_7_DAYS", "LAST_14_DAYS", "LAST_30_DAYS", "LAST_60_DAYS", "LAST_90_DAYS", "CUSTOM_N_DAYS")
		var days []string
		for _, day := range pulsemetric.ForkSupportedCustomDays() {
			days = append(days, strconv.Itoa(day))
		}
		helpmeta.Choices(command, "days", days...)
		helpmeta.FlagNote(command, "days", "required with --period CUSTOM_N_DAYS; otherwise omit")
		helpmeta.FlagNote(command, "period", "the source definition must allow the selected period's granularity")
		helpmeta.Value(command, "filter", "field=value")
		helpmeta.Value(command, "exclude-filter", "field=value")
	}
	if action == "list" {
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	}
	switch action {
	case "inspect", "delete", "followers":
		helpmeta.Required(command, "id")
	case "list":
		helpmeta.Required(command, "definition-id")
		helpmeta.Group(command, "exclusive", "all", "limit")
	case "fork":
		helpmeta.Required(command, "id")
		helpmeta.Group(command, "one-required", "period", "filter", "exclude-filter")
	case "follow":
		helpmeta.Required(command, "id")
		helpmeta.Group(command, "exactly-one", "user-id", "group-id")
	case "unfollow":
		helpmeta.Constraint(command, "Select --subscription-id alone, or --id with exactly one of --user-id or --group-id.")
	}
}

func metricHelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "pulse metric", Note: "Fork requires --id and at least one of --period, --filter, or --exclude-filter. CUSTOM_N_DAYS also requires --days.\nFollow requires --id and exactly one of --user-id or --group-id.", Common: false, Examples: []string{"tadx pulse metric fork --env dev --id <metric-luid> --filter <field>=<value> --filter <field>=<second-value> --preview"}},
		{Path: "pulse metric followers", Note: "", Common: false, Examples: []string{}},
	}
}
