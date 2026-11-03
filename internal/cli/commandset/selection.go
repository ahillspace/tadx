// Package commandset constructs only the command bindings needed by an invocation.
package commandset

import (
	"strings"

	"github.com/spf13/cobra"
)

// Selection is empty for discovery and ordinary command trees, or identifies the
// one action whose independent input bindings a batch row needs.
type Selection string

func Selected(ids []string) Selection {
	if len(ids) == 0 {
		return ""
	}
	return Selection(ids[0])
}

// Wants reports whether a capability or its containing family is selected.
func (s Selection) Wants(id string) bool {
	return s == "" || string(s) == id || strings.HasPrefix(string(s), id+".")
}

// Add constructs the selected action from the same binding used by the full tree.
func (s Selection) Add(parent *cobra.Command, id string, build func() *cobra.Command) {
	if s.Wants(id) {
		parent.AddCommand(build())
	}
}
