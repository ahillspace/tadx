package cli_test

import (
	"context"
	"reflect"
	"testing"

	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	"github.com/ahillspace/tadx/internal/cli"
	"github.com/spf13/cobra"
)

type lister struct {
	calls int
	input capabilitylist.Input
}

func (a *lister) Execute(_ context.Context, input capabilitylist.Input) (capabilitylist.Output, error) {
	a.calls++
	a.input = input
	return capabilitylist.Output{Capabilities: []capabilitylist.Capability{}}, nil
}

type getter struct{ id string }

func (a *getter) Execute(_ context.Context, input capabilityget.Input) (capabilityget.Output, error) {
	a.id = input.ID
	return capabilityget.Output{Capability: capabilityget.Capability{ID: input.ID}}, nil
}

type renderer struct{ values []any }

func (r *renderer) Render(value any) error {
	r.values = append(r.values, value)
	return nil
}

func TestRootExposesOnlyPhaseZeroExecutableCommands(t *testing.T) {
	r := &renderer{}
	cmd := cli.NewRoot(dependencies(&lister{}, &getter{}, r))

	var got []string
	for _, child := range cmd.Commands() {
		if child.Name() == "capability" {
			for _, grandchild := range child.Commands() {
				got = append(got, child.Name()+" "+grandchild.Name())
			}
		}
	}
	want := []string{"capability get", "capability list"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %v, want %v", got, want)
	}
}

func TestRegisteredCommandsComeFromCobraTree(t *testing.T) {
	cmd := cli.NewRoot(dependencies(&lister{}, &getter{}, &renderer{}))
	got, err := cli.RegisteredCommands(cmd)
	if err != nil {
		t.Fatal(err)
	}
	want := []cli.RegisteredCommand{
		{CapabilityID: "capability.get", CommandPath: []string{"capability", "get"}},
		{CapabilityID: "capability.list", CommandPath: []string{"capability", "list"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("registrations = %#v, want %#v", got, want)
	}
}

func TestRegisteredCommandsRejectsRunnableCommandWithoutCapabilityID(t *testing.T) {
	cmd := cli.NewRoot(dependencies(&lister{}, &getter{}, &renderer{}))
	cmd.AddCommand(&cobra.Command{Use: "unregistered", Run: func(*cobra.Command, []string) {}})
	if _, err := cli.RegisteredCommands(cmd); err == nil {
		t.Fatal("RegisteredCommands() error = nil")
	}
}

func TestRootDisablesDefaultCompletionCommand(t *testing.T) {
	cmd := cli.NewRoot(dependencies(&lister{}, &getter{}, &renderer{}))
	cmd.SetArgs([]string{"completion", "bash"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("completion command executed, want usage error")
	}
}

func TestCapabilityListInvokesActionAndRenderer(t *testing.T) {
	a := &lister{}
	r := &renderer{}
	cmd := cli.NewRoot(dependencies(a, &getter{}, r))
	cmd.SetArgs([]string{"capability", "list", "--domain", "content", "--resource", "workbook", "--owner", "cli", "--product", "cloud", "--limit", "5", "--cursor", "10"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if a.calls != 1 || len(r.values) != 1 {
		t.Fatalf("calls = %d, rendered = %d", a.calls, len(r.values))
	}
	if a.input.Domain != "content" || a.input.Resource != "workbook" || a.input.Owner != "cli" || a.input.Product != "cloud" || a.input.Limit != 5 || a.input.Cursor != "10" {
		t.Fatalf("input = %#v", a.input)
	}
}

func TestCapabilityGetRequiresExactlyOneID(t *testing.T) {
	g := &getter{}
	cmd := cli.NewRoot(dependencies(&lister{}, g, &renderer{}))
	cmd.SetArgs([]string{"capability", "get", "capability.list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if g.id != "capability.list" {
		t.Fatalf("ID = %q, want capability.list", g.id)
	}
}

func TestFullIsAUniversalPersistentPresentationFlag(t *testing.T) {
	for _, args := range [][]string{
		{"--full", "capability", "list"},
		{"capability", "list", "--full"},
	} {
		mode := &cli.RenderOptions{}
		deps := dependencies(&lister{}, &getter{}, &renderer{})
		deps.RenderOptions = mode
		cmd := cli.NewRoot(deps)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute(%v) error = %v", args, err)
		}
		if !mode.Full {
			t.Fatalf("Execute(%v) did not select full output", args)
		}
	}
}

func dependencies(l *lister, g *getter, r *renderer) cli.Dependencies {
	return cli.Dependencies{
		Lister:    l,
		Getter:    g,
		Renderer:  r,
		ListUse:   "list",
		ListShort: "List registered capabilities.",
		GetUse:    "get <id>",
		GetShort:  "Get one capability.",
	}
}
