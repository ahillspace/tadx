package update_test

import (
	"context"
	workbookupdate "github.com/ahillspace/tadx/actions/workbook/update"
	"github.com/ahillspace/tadx/internal/identity"
	"testing"
)

type resolver struct {
	item    workbookupdate.Workbook
	matches []workbookupdate.Workbook
}

func (r resolver) ResolveWorkbook(context.Context, identity.Selector) (workbookupdate.Workbook, error) {
	return r.item, nil
}
func (r resolver) FindWorkbooks(context.Context, string, string) ([]workbookupdate.Workbook, error) {
	return r.matches, nil
}

type updater struct {
	calls   int
	request workbookupdate.Request
}

func (u *updater) UpdateWorkbook(_ context.Context, r workbookupdate.Request) (workbookupdate.Result, error) {
	u.calls++
	u.request = r
	return workbookupdate.Result{Status: "succeeded", WorkbookLUID: r.LUID, WorkbookName: *r.Name, ProjectLUID: "p-1", OwnerLUID: "u-1"}, nil
}
func TestUpdatePreviewsThenRevalidatesExplicitRename(t *testing.T) {
	name := "New"
	r := resolver{item: workbookupdate.Workbook{LUID: "wb-1", Name: "Old", ProjectLUID: "p-1", OwnerLUID: "u-1"}}
	u := &updater{}
	a := workbookupdate.New(r, u)
	in := workbookupdate.Input{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "wb-1"}, Name: &name}
	if out, err := a.Execute(context.Background(), in, true); err != nil || out.Result != nil || u.calls != 0 {
		t.Fatalf("preview=%#v err=%v", out, err)
	}
	if out, err := a.Execute(context.Background(), in, false); err != nil || out.Result == nil || u.calls != 1 || u.request.Name == nil {
		t.Fatalf("execute=%#v err=%v request=%#v", out, err, u.request)
	}
}
func TestUpdateRejectsRenameCollision(t *testing.T) {
	name := "Taken"
	r := resolver{item: workbookupdate.Workbook{LUID: "wb-1", Name: "Old", ProjectLUID: "p-1"}, matches: []workbookupdate.Workbook{{LUID: "wb-2", Name: "Taken", ProjectLUID: "p-1"}}}
	if _, err := workbookupdate.New(r, &updater{}).Execute(context.Background(), workbookupdate.Input{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "wb-1"}, Name: &name}, false); err == nil {
		t.Fatal("expected collision error")
	}
}
