package version_test

import (
	"context"
	versionget "github.com/ahillspace/tadx/actions/version/get"
	versioncli "github.com/ahillspace/tadx/internal/cli/version"
	"testing"
)

type getter struct{ input versionget.Input }

func (g *getter) Execute(_ context.Context, input versionget.Input) (versionget.Output, error) {
	g.input = input
	return versionget.Output{}, nil
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
