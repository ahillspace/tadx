package helpmeta

import (
	"strings"

	"github.com/spf13/cobra"
)

// ExampleSet groups presentation facts around a registered command path.
type ExampleSet struct {
	Path     string
	Note     string
	Common   bool
	Examples []string
}

// AppendNote adds a command note idempotently without replacing existing help.
func AppendNote(command *cobra.Command, note string) {
	if note == "" || strings.Contains(command.Long, note) {
		return
	}
	if command.Long == "" {
		command.Long = command.Short
	}
	command.Long = strings.TrimSpace(command.Long) + "\n\n" + note
}
