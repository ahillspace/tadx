package update_test

import (
	"context"
	"errors"
	workbookupdate "github.com/ahillspace/tadx/actions/workbook/update"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
	"testing"
)

type descriptionResolver struct {
	calls int
	drift bool
}

func (r *descriptionResolver) ResolveWorkbook(context.Context, identity.Selector) (workbookupdate.Workbook, error) {
	r.calls++
	description := "old"
	if r.drift && r.calls > 1 {
		description = "other edit"
	}
	return workbookupdate.Workbook{LUID: "wb", Name: "Sales", ProjectLUID: "p", OwnerLUID: "u", Description: description}, nil
}
func (r *descriptionResolver) FindWorkbooks(context.Context, string, string) ([]workbookupdate.Workbook, error) {
	return nil, errors.New("description edits must not scan rename collisions")
}

type descriptionUpdater struct {
	calls   int
	request workbookupdate.Request
	fail    bool
}

func (u *descriptionUpdater) UpdateWorkbook(_ context.Context, r workbookupdate.Request) (workbookupdate.Result, error) {
	u.calls++
	u.request = r
	if u.fail {
		return workbookupdate.Result{Status: "unknown", WorkbookLUID: r.LUID}, errors.New("readback unavailable")
	}
	return workbookupdate.Result{Status: "succeeded", WorkbookLUID: r.LUID}, nil
}
func TestDescriptionChangesPreserveUntouchedPropertiesAndUnknownOutcome(t *testing.T) {
	for _, description := range []string{"new", "", "old"} {
		r := &descriptionResolver{}
		u := &descriptionUpdater{}
		out, err := workbookupdate.New(r, u).Execute(context.Background(), workbookupdate.Input{Environment: "test", Site: "site", Selector: identity.Selector{LUID: "wb"}, Description: &description}, false)
		if err != nil {
			t.Fatal(err)
		}
		if description == "old" {
			if !out.Plan.NoOp || u.calls != 0 {
				t.Fatal("no-op wrote")
			}
			continue
		}
		if u.calls != 1 || u.request.Name != nil || u.request.OwnerLUID != nil || u.request.Description == nil || *u.request.Description != description {
			t.Fatalf("request %#v", u.request)
		}
	}
	description := "new"
	u := &descriptionUpdater{fail: true}
	out, err := workbookupdate.New(&descriptionResolver{}, u).Execute(context.Background(), workbookupdate.Input{Environment: "test", Site: "site", Selector: identity.Selector{LUID: "wb"}, Description: &description}, false)
	if err == nil || out.Result == nil || out.Result.WorkbookLUID != "wb" || out.Result.Status != "unknown" {
		t.Fatalf("lost unknown result: %#v %v", out, err)
	}
	u = &descriptionUpdater{}
	_, err = workbookupdate.New(&descriptionResolver{drift: true}, u).Execute(context.Background(), workbookupdate.Input{Environment: "test", Site: "site", Selector: identity.Selector{LUID: "wb"}, Description: &description}, false)
	if err == nil || !strings.Contains(err.Error(), "changed") || u.calls != 0 {
		t.Fatalf("drift accepted: %v", err)
	}
}
