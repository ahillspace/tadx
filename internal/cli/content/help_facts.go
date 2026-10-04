package content

import (
	"strings"

	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// ApplyHelpFacts attaches the content command family's presentation contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "content" {
		helpmeta.Summary(command, "Workbooks, datasources, flows, and projects")
	}
	switch {
	case strings.HasPrefix(path, "content workbook "):
		workbookHelpFacts(command, strings.TrimPrefix(path, "content workbook "))
	case strings.HasPrefix(path, "content datasource "):
		datasourceHelpFacts(command, strings.TrimPrefix(path, "content datasource "))
	case strings.HasPrefix(path, "content flow "):
		flowHelpFacts(command, strings.TrimPrefix(path, "content flow "))
	case strings.HasPrefix(path, "content project "):
		projectHelpFacts(command, strings.TrimPrefix(path, "content project "))
	}
}
