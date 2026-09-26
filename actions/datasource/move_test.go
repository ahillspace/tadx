package datasource_test

import (
	"context"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/identity"
	"testing"
)

type moveResolver struct {
	item    datasourceops.Record
	project datasourceops.Project
	matches []datasourceops.Record
}

func (r moveResolver) ResolveDatasource(context.Context, identity.Selector) (datasourceops.Record, error) {
	return r.item, nil
}
func (r moveResolver) ResolveProject(context.Context, identity.Selector) (datasourceops.Project, error) {
	return r.project, nil
}
func (r moveResolver) FindDatasources(context.Context, string, string) ([]datasourceops.Record, error) {
	return r.matches, nil
}

type moveMover struct{ calls int }

func (m *moveMover) MoveDatasource(context.Context, string, string) (datasourceops.MoveResult, error) {
	m.calls++
	return datasourceops.MoveResult{Status: "succeeded", DatasourceLUID: "ds-1", ProjectLUID: "p-2"}, nil
}
func TestMoveMovePreviewsThenRevalidatesAndMoves(t *testing.T) {
	r := moveResolver{item: datasourceops.Record{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", OwnerLUID: "u-1"}, project: datasourceops.Project{LUID: "p-2"}}
	m := &moveMover{}
	aResolver, aMover := r, m
	in := datasourceops.MoveInput{Environment: "dev", Site: "site", DatasourceSelector: identity.Selector{LUID: "ds-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}
	if out, err := datasourceops.Move(context.Background(), aResolver, aMover, in, true); err != nil || out.Result != nil || m.calls != 0 {
		t.Fatalf("preview=%#v err=%v", out, err)
	}
	if out, err := datasourceops.Move(context.Background(), aResolver, aMover, in, false); err != nil || out.Result == nil || m.calls != 1 {
		t.Fatalf("execute=%#v err=%v", out, err)
	}
}
func TestMoveMoveRejectsCollision(t *testing.T) {
	r := moveResolver{item: datasourceops.Record{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1"}, project: datasourceops.Project{LUID: "p-2"}, matches: []datasourceops.Record{{LUID: "ds-2"}}}
	if _, err := datasourceops.Move(context.Background(), r, &moveMover{}, datasourceops.MoveInput{Environment: "dev", Site: "site", DatasourceSelector: identity.Selector{LUID: "ds-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}, false); err == nil {
		t.Fatal("expected collision error")
	}
}
