package capability_test

import (
	"context"
	"errors"
	"testing"

	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	cliCapability "github.com/ahillspace/tadx/internal/cli/capability"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/spf13/cobra"
)

type listAction struct {
	input capabilitylist.Input
	out   capabilitylist.Output
	err   error
	calls int
}

func (a *listAction) Execute(_ context.Context, input capabilitylist.Input) (capabilitylist.Output, error) {
	a.calls++
	a.input = input
	return a.out, a.err
}

type renderer struct{ values []any }

func (r *renderer) Render(value any) error {
	r.values = append(r.values, value)
	return nil
}

func TestListCarriesGlobalPresentationIntoContinuation(t *testing.T) {
	a := &listAction{}
	root := &cobra.Command{Use: "tadx"}
	root.PersistentFlags().Bool("full", false, "full")
	root.PersistentFlags().Bool("json", false, "json")
	root.AddCommand(cliCapability.New(cliCapability.Dependencies{
		Lister: a, Renderer: &renderer{}, ListUse: "list", ListShort: "list",
	}))
	root.SetArgs([]string{"capability", "list", "--full", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !a.input.Full || !a.input.JSON {
		t.Fatalf("presentation input = %#v", a.input)
	}
}

func TestListAllCarriesCompleteInventoryMode(t *testing.T) {
	a := &listAction{}
	root := &cobra.Command{Use: "tadx"}
	root.AddCommand(cliCapability.New(cliCapability.Dependencies{
		Lister: a, Renderer: &renderer{}, ListUse: "list", ListShort: "list",
	}))
	root.SetArgs([]string{"capability", "list", "--all"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !a.input.All || a.input.Limit != 0 || a.input.Cursor != "" {
		t.Fatalf("all input = %#v", a.input)
	}
}

func TestListAllRejectsLimitAndCursor(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "limit", args: []string{"--all", "--limit", "1"}},
		{name: "cursor", args: []string{"--all", "--cursor", "0"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := &listAction{}
			root := &cobra.Command{Use: "tadx"}
			root.AddCommand(cliCapability.New(cliCapability.Dependencies{
				Lister: a, Renderer: &renderer{}, ListUse: "list", ListShort: "list",
			}))
			root.SetArgs(append([]string{"capability", "list"}, test.args...))
			if err := root.Execute(); err == nil || a.calls != 0 {
				t.Fatalf("error = %v, calls = %d, want flag conflict before action", err, a.calls)
			}
		})
	}
}

func TestListReturnsStaticRowsWhenMutationPolicyIsUnavailable(t *testing.T) {
	policyErr := &errs.Error{ID: "configuration.load", Kind: errs.KindOperation, Operation: "configuration", Summary: "invalid profile"}
	a := &listAction{out: capabilitylist.Output{Capabilities: []capabilitylist.Capability{{ID: "capability.list"}}}}
	r := &renderer{}
	root := &cobra.Command{Use: "tadx"}
	root.AddCommand(cliCapability.New(cliCapability.Dependencies{
		Lister: a, Renderer: r, ListUse: "list", ListShort: "list",
		ResolveMutationPolicy: func(string) (bool, string, error) { return false, "", policyErr },
	}))
	root.SetArgs([]string{"capability", "list"})
	err := root.Execute()
	if err == nil || !errors.Is(err, policyErr) {
		t.Fatalf("error = %v, want policy error", err)
	}
	if a.input.MutationsEnabled || a.input.MutationPolicyUnavailable {
		t.Fatalf("partial input = %#v", a.input)
	}
	carrier, ok := err.(interface{ OperationOutput() any })
	if !ok {
		t.Fatalf("error does not carry output: %T", err)
	}
	partial, ok := carrier.OperationOutput().(capabilitylist.Output)
	if !ok || partial.MutationPolicy != "unavailable" {
		t.Fatalf("partial output = %#v", carrier.OperationOutput())
	}
	if len(r.values) != 0 {
		t.Fatalf("partial diagnostic was rendered before the error envelope: %#v", r.values)
	}
}
