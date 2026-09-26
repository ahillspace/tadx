package workbook_test

import (
	"context"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/identity"
	"testing"
)

type updateResolver struct {
	item    workbookops.Record
	matches []workbookops.Record
}

func (r updateResolver) ResolveWorkbook(context.Context, identity.Selector) (workbookops.Record, error) {
	return r.item, nil
}
func (r updateResolver) FindWorkbooks(context.Context, string, string) ([]workbookops.Record, error) {
	return r.matches, nil
}

type updateUpdater struct {
	calls   int
	request workbookops.UpdateRequest
}

func (u *updateUpdater) UpdateWorkbook(_ context.Context, r workbookops.UpdateRequest) (workbookops.UpdateResult, error) {
	u.calls++
	u.request = r
	return workbookops.UpdateResult{Status: "succeeded", WorkbookLUID: r.LUID, WorkbookName: *r.Name, ProjectLUID: "p-1", OwnerLUID: "u-1"}, nil
}
func TestUpdatePreviewsThenRevalidatesExplicitRename(t *testing.T) {
	name := "New"
	r := updateResolver{item: workbookops.Record{LUID: "wb-1", Name: "Old", ProjectLUID: "p-1", OwnerLUID: "u-1"}}
	u := &updateUpdater{}
	aResolver, aUpdater := r, u
	in := workbookops.UpdateInput{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "wb-1"}, Name: &name}
	if out, err := workbookops.Update(context.Background(), aResolver, aUpdater, in, true); err != nil || out.Result != nil || u.calls != 0 {
		t.Fatalf("preview=%#v err=%v", out, err)
	}
	if out, err := workbookops.Update(context.Background(), aResolver, aUpdater, in, false); err != nil || out.Result == nil || u.calls != 1 || u.request.Name == nil {
		t.Fatalf("execute=%#v err=%v request=%#v", out, err, u.request)
	}
}
func TestUpdateRejectsRenameCollision(t *testing.T) {
	name := "Taken"
	r := updateResolver{item: workbookops.Record{LUID: "wb-1", Name: "Old", ProjectLUID: "p-1"}, matches: []workbookops.Record{{LUID: "wb-2", Name: "Taken", ProjectLUID: "p-1"}}}
	if _, err := workbookops.Update(context.Background(), r, &updateUpdater{}, workbookops.UpdateInput{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "wb-1"}, Name: &name}, false); err == nil {
		t.Fatal("expected collision error")
	}
}
