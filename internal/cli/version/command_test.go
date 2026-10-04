package version_test

import (
	"context"
	versionaction "github.com/ahillspace/tadx/actions/version"
	versioncli "github.com/ahillspace/tadx/internal/cli/version"
	"testing"
)

type getter struct{ input versionaction.Input }

func (g *getter) Get(_ context.Context, input versionaction.Input) (versionaction.Output, error) {
	g.input = input
	return versionaction.Output{}, nil
}

type renderer struct{}

func (renderer) Render(any) error { return nil }
func TestCommandMapsCheck(t *testing.T) {
	g := &getter{}
	command := versioncli.New(versioncli.Dependencies{Getter: g, Renderer: renderer{}})
	command.SetArgs([]string{"--check"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !g.input.Check {
		t.Fatal("--check was not mapped")
	}
}
