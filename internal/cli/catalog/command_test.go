package catalog_test

import (
	"context"
	"reflect"
	"testing"

	catalogrefresh "github.com/ahillspace/tadx/actions/catalog/refresh"
	catalogstatus "github.com/ahillspace/tadx/actions/catalog/status"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
)

type actions struct {
	refreshInputs []catalogrefresh.Input
	statusInputs  []catalogstatus.Input
}

type refresher struct{ actions *actions }

func (r refresher) Execute(_ context.Context, input catalogrefresh.Input) (catalogrefresh.Output, error) {
	r.actions.refreshInputs = append(r.actions.refreshInputs, input)
	return catalogrefresh.Output{}, nil
}

type statuser struct{ actions *actions }

func (s statuser) Execute(_ context.Context, input catalogstatus.Input) (catalogstatus.Output, error) {
	s.actions.statusInputs = append(s.actions.statusInputs, input)
	return catalogstatus.Output{}, nil
}

type renderer struct{}

func (renderer) Render(any) error { return nil }

func TestCatalogMountsRefreshAndStatusWithBoundedInputs(t *testing.T) {
	recorded := &actions{}
	command := catalogcli.New(catalogcli.Dependencies{
		Refresher: refresher{recorded}, Statuser: statuser{recorded}, Renderer: renderer{},
	})

	command.SetArgs([]string{"refresh", "--environment", "production", "--site", "marketing"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var wantScopes []string
	if len(recorded.refreshInputs) != 1 || !reflect.DeepEqual(recorded.refreshInputs[0].Scopes, wantScopes) {
		t.Fatalf("refresh inputs = %#v", recorded.refreshInputs)
	}

	command.SetArgs([]string{"refresh", "--environment", "production", "--scope", "workbooks", "--scope", "permissions"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	wantScopes = []string{"workbooks", "permissions"}
	if len(recorded.refreshInputs) != 2 || !reflect.DeepEqual(recorded.refreshInputs[1].Scopes, wantScopes) {
		t.Fatalf("scoped refresh inputs = %#v", recorded.refreshInputs)
	}

	command.SetArgs([]string{"status", "--environment", "production", "--site", "marketing"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(recorded.statusInputs) != 1 || recorded.statusInputs[0].Environment != "production" || recorded.statusInputs[0].Site != "marketing" {
		t.Fatalf("status inputs = %#v", recorded.statusInputs)
	}
}

func TestCatalogDoesNotMountGatedContentCommands(t *testing.T) {
	command := catalogcli.New(catalogcli.Dependencies{Refresher: refresher{&actions{}}, Statuser: statuser{&actions{}}, Renderer: renderer{}})
	for _, name := range []string{"refresh", "status"} {
		if child, _, err := command.Find([]string{name}); err != nil || child == command || child.Name() != name {
			t.Fatalf("catalog command %q missing: child=%v err=%v", name, child, err)
		}
	}
	for _, name := range []string{"search", "get", "content-search", "content-get"} {
		if child, _, err := command.Find([]string{name}); err == nil && child != command {
			t.Fatalf("gated command %q was mounted", name)
		}
	}
}
