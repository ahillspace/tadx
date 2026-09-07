package cli_test

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	authstatus "github.com/ahillspace/tadx/actions/auth/status"
	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	"github.com/ahillspace/tadx/internal/cli"
	envcli "github.com/ahillspace/tadx/internal/cli/env"
	workspacecli "github.com/ahillspace/tadx/internal/cli/workspace"
	"github.com/ahillspace/tadx/internal/errs"
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

type noMutationPolicy struct{}

func (noMutationPolicy) IsRemoteMutation(string) bool { return false }

type authChecker struct{}

func (authChecker) Execute(context.Context, authcheck.Input) (authcheck.Output, error) {
	return authcheck.Output{}, nil
}

type authStatuser struct{}

func (authStatuser) Execute(context.Context, authstatus.Input) (authstatus.Output, error) {
	return authstatus.Output{}, nil
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

func TestRootAddsEnvironmentAndAuthStatusCommands(t *testing.T) {
	profiles := &envcli.Dependencies{}
	deps := dependencies(&lister{}, &getter{}, &renderer{})
	deps.EnvironmentProfiles = profiles
	deps.Workspaces = &workspacecli.Dependencies{}
	deps.AuthChecker = authChecker{}
	deps.AuthStatuser = authStatuser{}
	root := cli.NewRoot(deps)
	registrations, err := cli.RegisteredCommands(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, registration := range registrations {
		if registration.CapabilityID == "auth.status" || len(registration.CommandPath) > 0 && (registration.CommandPath[0] == "env" || registration.CommandPath[0] == "workspace") {
			got = append(got, registration.CapabilityID)
		}
	}
	want := []string{"auth.status", "env.profile.add", "env.profile.set-default", "env.profile.get", "env.profile.list", "env.profile.remove", "env.profile.update", "workspace.artifact.delete", "workspace.move", "workspace.clean", "workspace.clone", "workspace.create", "workspace.delete", "workspace.list", "workspace.register", "workspace.set-default", "workspace.status", "workspace.unregister"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("registrations = %v, want %v", got, want)
	}
}

func TestRootEnablesUnregisteredLocalCompletionCommand(t *testing.T) {
	cmd := cli.NewRoot(dependencies(&lister{}, &getter{}, &renderer{}))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"completion", "bash"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "__start_tadx") {
		t.Fatalf("completion output = %q", output.String())
	}
	completion, _, err := cmd.Find([]string{"completion"})
	if err != nil {
		t.Fatal(err)
	}
	if completion.Annotations["tadx.capability"] != "" || completion.Annotations["tadx.grouping"] != "true" {
		t.Fatalf("completion annotations = %#v", completion.Annotations)
	}
}

func TestGroupingCommandRejectsUnknownChildWithUsageExit(t *testing.T) {
	deps := dependencies(&lister{}, &getter{}, &renderer{})
	deps.Workspaces = &workspacecli.Dependencies{}
	command := cli.NewRoot(deps)
	command.SetArgs([]string{"workspace", "not-a-command"})
	err := command.Execute()
	if err == nil || errs.ExitCode(err) != 2 {
		t.Fatalf("error=%v exit=%d", err, errs.ExitCode(err))
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

func TestConfigIsAUniversalPersistentPathFlag(t *testing.T) {
	for _, args := range [][]string{
		{"--config", "portable/config.yaml", "capability", "list"},
		{"capability", "list", "--config", "portable/config.yaml"},
	} {
		path := ""
		deps := dependencies(&lister{}, &getter{}, &renderer{})
		deps.ConfigPath = &path
		cmd := cli.NewRoot(deps)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute(%v) error = %v", args, err)
		}
		if path != "portable/config.yaml" {
			t.Fatalf("Execute(%v) config path = %q", args, path)
		}
	}
}

func dependencies(l *lister, g *getter, r *renderer) cli.Dependencies {
	return cli.Dependencies{
		Lister:         l,
		Getter:         g,
		Renderer:       r,
		MutationPolicy: noMutationPolicy{},
		ListUse:        "list",
		ListShort:      "List registered capabilities.",
		GetUse:         "get <id>",
		GetShort:       "Get one capability.",
	}
}
