package content

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
	"strings"
)

// ApplyHelpFacts attaches this command family's presentation-only contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "content" {
		helpmeta.Summary(command, "Workbooks, datasources, flows, and projects")
	}
	switch path {
	case "content datasource schema":
		helpmeta.Choices(command, "role", "measure", "dimension", "date", "excluded")
		helpmeta.FlagNote(command, "field-id", "at most 10000 distinct identifiers")
	case "content project create", "content project update":
		helpmeta.Choices(command, "content-permissions", "ManagedByOwner", "LockedToProject", "LockedToProjectWithoutNested")
	case "content datasource list":
		helpmeta.Value(command, "updated-after", "RFC3339")
		helpmeta.Value(command, "updated-before", "RFC3339")
		helpmeta.FlagNote(command, "type", "exact provider datasource type; not a search resource family")
	}
	switch path {
	case "content workbook list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	case "content datasource list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	case "content flow list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	case "content project list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	case "content datasource schema":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"20"})
	}
	switch path {
	case "content datasource schema":
		helpmeta.Required(command, "id")
	case "content project create":
		helpmeta.Required(command, "name")
		helpmeta.Group(command, "exclusive", "parent-id", "parent")
	case "content project inspect":
		helpmeta.Group(command, "exactly-one", "id", "project-id", "project")
	case "content project delete":
		helpmeta.Group(command, "exactly-one", "id", "project-id")
	case "content project update":
		helpmeta.Group(command, "exactly-one", "id", "project-id", "project")
		helpmeta.Group(command, "one-required", "new-name", "name", "description", "content-permissions")
	case "content project move":
		helpmeta.Group(command, "exactly-one", "id", "project-id", "project")
		helpmeta.Group(command, "exactly-one", "parent-id", "parent", "top-level")
	}
	for _, resource := range []string{"workbook", "datasource", "flow"} {
		prefix := "content " + resource + " "
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		action := strings.TrimPrefix(path, prefix)
		switch action {
		case "inspect", "pull", "update", "move", "delete":
			helpmeta.Group(command, "exactly-one", "id", "name")
			if action == "inspect" && command.Flags().Lookup("project-id") != nil {
				helpmeta.Constraint(command, "--name requires exactly one of --project or --project-id; --id excludes name/project selectors.")
			} else if resource == "workbook" && action == "pull" {
				helpmeta.Constraint(command, "--project scopes an exact --name selector when supplied.")
			} else {
				helpmeta.Constraint(command, "--name requires --project; --id excludes both.")
			}
		case "publish":
			helpmeta.Group(command, "exactly-one", "artifact", "file", "id", "artifact-name")
			helpmeta.Group(command, "exactly-one", "project-id", "project")
			if resource == "datasource" {
				helpmeta.Group(command, "exactly-one", "create", "overwrite", "append", "replace")
			}
		}
		if action == "move" {
			helpmeta.Group(command, "exactly-one", "destination-project-id", "destination-project")
		}
		if action == "update" {
			switch resource {
			case "workbook":
				helpmeta.Group(command, "one-required", "new-name", "owner-id", "description")
			case "datasource":
				helpmeta.Group(command, "one-required", "new-name", "owner-id")
			case "flow":
				helpmeta.Required(command, "owner-id")
			}
		}
	}
}
