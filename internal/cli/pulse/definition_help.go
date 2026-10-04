package pulse

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

func definitionHelpFacts(command *cobra.Command, action string) {
	if action == "create" {
		helpmeta.Choices(command, "aggregation", "SUM", "AVERAGE", "MIN", "MAX", "COUNT", "COUNT_DISTINCT", "USER")
		helpmeta.Choices(command, "minimum-granularity", "DAY", "WEEK", "MONTH", "QUARTER", "YEAR")
		helpmeta.Choices(command, "number-format", "NUMBER", "CURRENCY", "PERCENT")
		helpmeta.Choices(command, "sentiment", "UP", "DOWN", "NONE")
		helpmeta.Choices(command, "temporality", "OVER_TIME", "LATEST")
		helpmeta.Value(command, "currency", "AAA")
		helpmeta.FlagNote(command, "name", "at most 255 Unicode characters")
		helpmeta.FlagNote(command, "description", "at most 1024 Unicode characters")
	}
	if action == "publish" {
		helpmeta.Value(command, "datasource-map", "source-luid=destination-luid")
	}
	if action == "list" {
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	}
	switch action {
	case "create":
		helpmeta.Required(command, "name", "datasource-id", "measure-field", "date-field", "dimension")
		helpmeta.Constraint(command, "--running-total requires --aggregation SUM and --temporality OVER_TIME; currency applies to --number-format CURRENCY.")
	case "inspect", "pull", "delete":
		helpmeta.Required(command, "id")
	case "publish":
		helpmeta.Required(command, "datasource-map")
		helpmeta.Group(command, "exactly-one", "artifact", "id", "artifact-name")
	case "list":
		helpmeta.Group(command, "exclusive", "all", "limit")
	}
}

func definitionHelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "pulse definition", Note: "Create requires --name, --datasource-id, --measure-field, and --date-field. Repeat --dimension for allowed slicers.\nPublish requires one of --artifact, --id, or --artifact-name, plus explicit --datasource-map source=destination mappings.", Common: false, Examples: []string{"tadx pulse definition publish --env dev --workspace dev --id <definition-luid> --datasource-map <source-luid>=<destination-luid> --preview"}},
	}
}
