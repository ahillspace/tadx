package pulse

import (
	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
	"strconv"
)

// ApplyHelpFacts attaches this command family's presentation-only contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "pulse" {
		helpmeta.Summary(command, "Definitions, metric variants, and followers")
	}
	switch path {
	case "pulse definition create":
		helpmeta.Choices(command, "aggregation", "SUM", "AVERAGE", "MIN", "MAX", "COUNT", "COUNT_DISTINCT", "USER")
		helpmeta.Choices(command, "minimum-granularity", "DAY", "WEEK", "MONTH", "QUARTER", "YEAR")
		helpmeta.Choices(command, "number-format", "NUMBER", "CURRENCY", "PERCENT")
		helpmeta.Choices(command, "sentiment", "UP", "DOWN", "NONE")
		helpmeta.Choices(command, "temporality", "OVER_TIME", "LATEST")
		helpmeta.Value(command, "currency", "AAA")
		helpmeta.FlagNote(command, "name", "at most 255 Unicode characters")
		helpmeta.FlagNote(command, "description", "at most 1024 Unicode characters")
	case "pulse metric fork":
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
	case "pulse definition publish":
		helpmeta.Value(command, "datasource-map", "source-luid=destination-luid")
	}
	switch path {
	case "pulse definition list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	case "pulse metric list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	}
	switch path {
	case "pulse definition create":
		helpmeta.Required(command, "name", "datasource-id", "measure-field", "date-field", "dimension")
		helpmeta.Constraint(command, "--running-total requires --aggregation SUM and --temporality OVER_TIME; currency applies to --number-format CURRENCY.")
	case "pulse definition inspect", "pulse definition pull", "pulse definition delete", "pulse metric inspect", "pulse metric delete", "pulse metric followers":
		helpmeta.Required(command, "id")
	case "pulse definition publish":
		helpmeta.Required(command, "datasource-map")
		helpmeta.Group(command, "exactly-one", "artifact", "id", "artifact-name")
	case "pulse metric list":
		helpmeta.Required(command, "definition-id")
		helpmeta.Group(command, "exclusive", "all", "limit")
	case "pulse definition list":
		helpmeta.Group(command, "exclusive", "all", "limit")
	case "pulse metric fork":
		helpmeta.Required(command, "id")
		helpmeta.Group(command, "one-required", "period", "filter", "exclude-filter")
	case "pulse metric follow":
		helpmeta.Required(command, "id")
		helpmeta.Group(command, "exactly-one", "user-id", "group-id")
	case "pulse metric unfollow":
		helpmeta.Constraint(command, "Select --subscription-id alone, or --id with exactly one of --user-id or --group-id.")
	}
}
