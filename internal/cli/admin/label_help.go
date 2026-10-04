package admin

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

func labelValueHelpFacts(command *cobra.Command, action string) {
	if action == "list" {
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"20"})
	}
	switch action {
	case "inspect", "delete":
		helpmeta.Required(command, "name")
	case "update":
		helpmeta.Required(command, "name")
		helpmeta.Group(command, "one-required", "new-name", "category", "description")
		helpmeta.Constraint(command, "Creating a missing label value requires --category and --description; renaming requires an existing value.")
	}
}

func labelCategoryHelpFacts(command *cobra.Command, action string) {
	if action == "list" {
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"20"})
	}
	switch action {
	case "inspect", "delete":
		helpmeta.Required(command, "name")
	case "create":
		helpmeta.Required(command, "name", "description")
	case "update":
		helpmeta.Required(command, "name")
		helpmeta.Group(command, "one-required", "new-name", "description")
	}
}

func labelHelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "admin label", Note: "Shared label definitions are separate from labels attached to assets under catalog label.", Common: true, Examples: []string{}},
		{Path: "admin label-value", Note: "Inspect, update, and delete select an exact --name. Creating a value through update also requires --category.", Common: false, Examples: []string{}},
		{Path: "admin label-category", Note: "Create, inspect, update, and delete select an exact --name.", Common: false, Examples: []string{}},
	}
}
