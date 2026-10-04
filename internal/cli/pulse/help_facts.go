package pulse

import (
	"strings"

	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// ApplyHelpFacts attaches the Pulse command family's presentation contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "pulse" {
		helpmeta.Summary(command, "Definitions, metric variants, and followers")
	}
	switch {
	case strings.HasPrefix(path, "pulse definition "):
		definitionHelpFacts(command, strings.TrimPrefix(path, "pulse definition "))
	case strings.HasPrefix(path, "pulse metric "):
		metricHelpFacts(command, strings.TrimPrefix(path, "pulse metric "))
	}
}
