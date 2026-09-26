package workbook_test

import (
	"context"
	"errors"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
	"testing"
)

type updateDescriptionResolver struct {
	calls int
	drift bool
}

func (r *updateDescriptionResolver) ResolveWorkbook(context.Context, identity.Selector) (workbookops.Record, error) {
	r.calls++
	description := "old"
	if r.drift && r.calls > 1 {
		description = "other edit"
	}
	return workbookops.Record{LUID: "wb", Name: "Sales", ProjectLUID: "p", OwnerLUID: "u", Description: description}, nil
}
func (r *updateDescriptionResolver) FindWorkbooks(context.Context, string, string) ([]workbookops.Record, error) {
	return nil, errors.New("description edits must not scan rename collisions")
}

type updateDescriptionUpdater struct {
	calls   int
	request workbookops.UpdateRequest
	fail    bool
}

func (u *updateDescriptionUpdater) UpdateWorkbook(_ context.Context, r workbookops.UpdateRequest) (workbookops.UpdateResult, error) {
	u.calls++
	u.request = r
	if u.fail {
		return workbookops.UpdateResult{Status: "unknown", WorkbookLUID: r.LUID}, errors.New("readback unavailable")
	}
	return workbookops.UpdateResult{Status: "succeeded", WorkbookLUID: r.LUID}, nil
}
func TestUpdateDescriptionChangesPreserveUntouchedPropertiesAndUnknownOutcome(t *testing.T) {
	for _, description := range []string{"new", "", "old"} {
		r := &updateDescriptionResolver{}
		u := &updateDescriptionUpdater{}
		out, err := workbookops.Update(context.Background(), r, u, workbookops.UpdateInput{Environment: "test", Site: "site", Selector: identity.Selector{LUID: "wb"}, Description: &description}, false)
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
	u := &updateDescriptionUpdater{fail: true}
	out, err := workbookops.Update(context.Background(), &updateDescriptionResolver{}, u, workbookops.UpdateInput{Environment: "test", Site: "site", Selector: identity.Selector{LUID: "wb"}, Description: &description}, false)
	if err == nil || out.Result == nil || out.Result.WorkbookLUID != "wb" || out.Result.Status != "unknown" {
		t.Fatalf("lost unknown result: %#v %v", out, err)
	}
	u = &updateDescriptionUpdater{}
	_, err = workbookops.Update(context.Background(), &updateDescriptionResolver{drift: true}, u, workbookops.UpdateInput{Environment: "test", Site: "site", Selector: identity.Selector{LUID: "wb"}, Description: &description}, false)
	if err == nil || !strings.Contains(err.Error(), "changed") || u.calls != 0 {
		t.Fatalf("drift accepted: %v", err)
	}
}
