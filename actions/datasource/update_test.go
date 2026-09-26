package datasource_test

import (
	"context"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/identity"
	"testing"
)

type updateResolver struct {
	item    datasourceops.Record
	matches []datasourceops.Record
}

func (r updateResolver) ResolveDatasource(context.Context, identity.Selector) (datasourceops.Record, error) {
	return r.item, nil
}
func (r updateResolver) FindDatasources(context.Context, string, string) ([]datasourceops.Record, error) {
	return r.matches, nil
}

type updateUpdater struct {
	calls   int
	request datasourceops.UpdateRequest
}

func (u *updateUpdater) UpdateDatasource(_ context.Context, r datasourceops.UpdateRequest) (datasourceops.UpdateResult, error) {
	u.calls++
	u.request = r
	return datasourceops.UpdateResult{Status: "succeeded", DatasourceLUID: r.LUID, DatasourceName: *r.Name, ProjectLUID: "p-1", OwnerLUID: "u-1"}, nil
}
func TestUpdateUpdatePreviewsThenRevalidatesRename(t *testing.T) {
	name := "New"
	r := updateResolver{item: datasourceops.Record{LUID: "ds-1", Name: "Old", ProjectLUID: "p-1", OwnerLUID: "u-1"}}
	u := &updateUpdater{}
	aResolver, aUpdater := r, u
	in := datasourceops.UpdateInput{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "ds-1"}, Name: &name}
	if out, err := datasourceops.Update(context.Background(), aResolver, aUpdater, in, true); err != nil || out.Result != nil || u.calls != 0 {
		t.Fatalf("preview=%#v err=%v", out, err)
	}
	if out, err := datasourceops.Update(context.Background(), aResolver, aUpdater, in, false); err != nil || out.Result == nil || u.calls != 1 {
		t.Fatalf("execute=%#v err=%v", out, err)
	}
}
func TestUpdateUpdateRejectsRenameCollision(t *testing.T) {
	name := "Taken"
	r := updateResolver{item: datasourceops.Record{LUID: "ds-1", Name: "Old", ProjectLUID: "p-1"}, matches: []datasourceops.Record{{LUID: "ds-2"}}}
	if _, err := datasourceops.Update(context.Background(), r, &updateUpdater{}, datasourceops.UpdateInput{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "ds-1"}, Name: &name}, false); err == nil {
		t.Fatal("expected collision error")
	}
}
