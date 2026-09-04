package admin_test

import (
	"testing"

	cli "github.com/ahillspace/tadx/internal/cli/admin"
	"github.com/spf13/cobra"
)

func TestCatalogFlagIsLimitedToEligibleAdminReads(t *testing.T) {
	root := cli.New(deps(&fake{}, true))
	for _, path := range [][]string{{"user", "list"}, {"user", "get"}, {"group", "list"}, {"group", "get"}} {
		if commandAt(root, path...).Flags().Lookup("catalog") == nil {
			t.Errorf("%v is missing --catalog", path)
		}
	}
	for _, path := range [][]string{{"user", "create"}, {"user", "update"}, {"user", "delete"}, {"group", "create"}, {"group", "update"}, {"group", "delete"}, {"permission", "get"}} {
		if commandAt(root, path...).Flags().Lookup("catalog") != nil {
			t.Errorf("%v must remain live-only", path)
		}
	}
}

func commandAt(root *cobra.Command, path ...string) *cobra.Command {
	current := root
	for _, name := range path {
		found := &cobra.Command{}
		for _, command := range current.Commands() {
			if command.Name() == name {
				found = command
				break
			}
		}
		current = found
	}
	return current
}
