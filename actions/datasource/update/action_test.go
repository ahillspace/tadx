package update_test

import (
	"context"
	datasourceupdate "github.com/ahillspace/tadx/actions/datasource/update"
	"github.com/ahillspace/tadx/internal/identity"
	"testing"
)

type resolver struct {
	item    datasourceupdate.Datasource
	matches []datasourceupdate.Datasource
}

func (r resolver) ResolveDatasource(context.Context, identity.Selector) (datasourceupdate.Datasource, error) {
	return r.item, nil
}
func (r resolver) FindDatasources(context.Context, string, string) ([]datasourceupdate.Datasource, error) {
	return r.matches, nil
}

type updater struct {
	calls   int
	request datasourceupdate.Request
}

func (u *updater) UpdateDatasource(_ context.Context, r datasourceupdate.Request) (datasourceupdate.Result, error) {
	u.calls++
	u.request = r
	return datasourceupdate.Result{Status: "succeeded", DatasourceLUID: r.LUID, DatasourceName: *r.Name, ProjectLUID: "p-1", OwnerLUID: "u-1"}, nil
}
func TestUpdatePreviewsThenRevalidatesRename(t *testing.T) {
	name := "New"
	r := resolver{item: datasourceupdate.Datasource{LUID: "ds-1", Name: "Old", ProjectLUID: "p-1", OwnerLUID: "u-1"}}
	u := &updater{}
	a := datasourceupdate.New(r, u)
	in := datasourceupdate.Input{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "ds-1"}, Name: &name}
	if out, err := a.Execute(context.Background(), in, true); err != nil || out.Result != nil || u.calls != 0 {
		t.Fatalf("preview=%#v err=%v", out, err)
	}
	if out, err := a.Execute(context.Background(), in, false); err != nil || out.Result == nil || u.calls != 1 {
		t.Fatalf("execute=%#v err=%v", out, err)
	}
}
func TestUpdateRejectsRenameCollision(t *testing.T) {
	name := "Taken"
	r := resolver{item: datasourceupdate.Datasource{LUID: "ds-1", Name: "Old", ProjectLUID: "p-1"}, matches: []datasourceupdate.Datasource{{LUID: "ds-2"}}}
	if _, err := datasourceupdate.New(r, &updater{}).Execute(context.Background(), datasourceupdate.Input{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "ds-1"}, Name: &name}, false); err == nil {
		t.Fatal("expected collision error")
	}
}
