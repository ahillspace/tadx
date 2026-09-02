package catalog_test

import (
	"context"
	"reflect"
	"testing"

	catalogget "github.com/ahillspace/tadx/actions/catalog/get"
	catalogrefresh "github.com/ahillspace/tadx/actions/catalog/refresh"
	catalogsearch "github.com/ahillspace/tadx/actions/catalog/search"
	catalogstatus "github.com/ahillspace/tadx/actions/catalog/status"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
)

type actions struct {
	refreshInputs []catalogrefresh.Input
	getInputs     []catalogget.Input
	statusInputs  []catalogstatus.Input
}

type refresher struct{ actions *actions }

func (r refresher) Execute(_ context.Context, input catalogrefresh.Input) (catalogrefresh.Output, error) {
	r.actions.refreshInputs = append(r.actions.refreshInputs, input)
	return catalogrefresh.Output{}, nil
}

type getter struct{ actions *actions }

func (g getter) Execute(_ context.Context, input catalogget.Input) (catalogget.Output, error) {
	g.actions.getInputs = append(g.actions.getInputs, input)
	return catalogget.Output{}, nil
}

type statuser struct{ actions *actions }

func (s statuser) Execute(_ context.Context, input catalogstatus.Input) (catalogstatus.Output, error) {
	s.actions.statusInputs = append(s.actions.statusInputs, input)
	return catalogstatus.Output{}, nil
}

type searcher struct{}

func (searcher) Execute(context.Context, catalogsearch.Input) (catalogsearch.Output, error) {
	return catalogsearch.Output{}, nil
}

type renderer struct{}

func (renderer) Render(any) error { return nil }

func TestCatalogMountsRefreshGetAndStatusWithBoundedInputs(t *testing.T) {
	recorded := &actions{}
	command := catalogcli.New(catalogcli.Dependencies{
		Searcher: searcher{}, Refresher: refresher{recorded}, Getter: getter{recorded}, Statuser: statuser{recorded}, Renderer: renderer{},
	})

	command.SetArgs([]string{"refresh", "--environment", "production", "--site", "marketing"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	wantScopes := []string{"projects", "workbooks", "datasources", "flows"}
	if len(recorded.refreshInputs) != 1 || !reflect.DeepEqual(recorded.refreshInputs[0].Scopes, wantScopes) {
		t.Fatalf("refresh inputs = %#v", recorded.refreshInputs)
	}

	command.SetArgs([]string{"get", "--environment", "production", "--site", "marketing", "--kind", "workbook", "--id", "wb-1"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(recorded.getInputs) != 1 || recorded.getInputs[0].LUID != "wb-1" || recorded.getInputs[0].Kind != "workbook" {
		t.Fatalf("get inputs = %#v", recorded.getInputs)
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
	command := catalogcli.New(catalogcli.Dependencies{Searcher: searcher{}, Refresher: refresher{&actions{}}, Getter: getter{&actions{}}, Statuser: statuser{&actions{}}, Renderer: renderer{}})
	for _, name := range []string{"search", "refresh", "get", "status"} {
		if child, _, err := command.Find([]string{name}); err != nil || child == command || child.Name() != name {
			t.Fatalf("catalog command %q missing: child=%v err=%v", name, child, err)
		}
	}
	for _, name := range []string{"content-search", "content-get"} {
		if child, _, err := command.Find([]string{name}); err == nil && child != command {
			t.Fatalf("gated command %q was mounted", name)
		}
	}
}
