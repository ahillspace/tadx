package cli_test

import (
	"github.com/ahillspace/tadx/internal/cli"
	"reflect"
	"testing"
)

func TestRootRegistersOnlySuppliedCommandDependencies(t *testing.T) {
	deps := dependencies(&lister{}, &getter{}, &renderer{})
	deps.AuthChecker = &checker{}
	deps.Searcher = &searcher{}
	deps.WorkbookPuller = &puller{}
	deps.WorkbookPublisher = &publisher{}
	root := cli.NewRoot(deps)
	registered, err := cli.RegisteredCommands(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []cli.RegisteredCommand{
		{CapabilityID: "auth.check", CommandPath: []string{"auth", "check"}},
		{CapabilityID: "capability.get", CommandPath: []string{"capability", "get"}},
		{CapabilityID: "capability.list", CommandPath: []string{"capability", "list"}},
		{CapabilityID: "workbook.publish", CommandPath: []string{"content", "workbook", "publish"}},
		{CapabilityID: "workbook.pull", CommandPath: []string{"content", "workbook", "pull"}},
		{CapabilityID: "search.run", CommandPath: []string{"search"}},
	}
	if !reflect.DeepEqual(registered, want) {
		t.Fatalf("registered = %#v, want %#v", registered, want)
	}
}
