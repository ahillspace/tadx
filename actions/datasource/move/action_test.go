package move_test

import (
	"context"
	datasourcemove "github.com/ahillspace/tadx/actions/datasource/move"
	"github.com/ahillspace/tadx/internal/identity"
	"testing"
)

type resolver struct {
	item    datasourcemove.Datasource
	project datasourcemove.Project
	matches []datasourcemove.Datasource
}

func (r resolver) ResolveDatasource(context.Context, identity.Selector) (datasourcemove.Datasource, error) {
	return r.item, nil
}
func (r resolver) ResolveProject(context.Context, identity.Selector) (datasourcemove.Project, error) {
	return r.project, nil
}
func (r resolver) FindDatasources(context.Context, string, string) ([]datasourcemove.Datasource, error) {
	return r.matches, nil
}

type mover struct{ calls int }

func (m *mover) MoveDatasource(context.Context, string, string) (datasourcemove.Result, error) {
	m.calls++
	return datasourcemove.Result{Status: "succeeded", DatasourceLUID: "ds-1", ProjectLUID: "p-2"}, nil
}
func TestMovePreviewsThenRevalidatesAndMoves(t *testing.T) {
	r := resolver{item: datasourcemove.Datasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", OwnerLUID: "u-1"}, project: datasourcemove.Project{LUID: "p-2"}}
	m := &mover{}
	a := datasourcemove.New(r, m)
	in := datasourcemove.Input{Environment: "dev", Site: "site", DatasourceSelector: identity.Selector{LUID: "ds-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}
	if out, err := a.Execute(context.Background(), in, true); err != nil || out.Result != nil || m.calls != 0 {
		t.Fatalf("preview=%#v err=%v", out, err)
	}
	if out, err := a.Execute(context.Background(), in, false); err != nil || out.Result == nil || m.calls != 1 {
		t.Fatalf("execute=%#v err=%v", out, err)
	}
}
func TestMoveRejectsCollision(t *testing.T) {
	r := resolver{item: datasourcemove.Datasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1"}, project: datasourcemove.Project{LUID: "p-2"}, matches: []datasourcemove.Datasource{{LUID: "ds-2"}}}
	if _, err := datasourcemove.New(r, &mover{}).Execute(context.Background(), datasourcemove.Input{Environment: "dev", Site: "site", DatasourceSelector: identity.Selector{LUID: "ds-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}, false); err == nil {
		t.Fatal("expected collision error")
	}
}
