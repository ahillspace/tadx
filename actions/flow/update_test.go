package flow_test

import (
	"context"
	flowupdate "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/identity"
	"testing"
)

type updateResolver struct{ item flowupdate.Record }

func (r updateResolver) ResolveFlow(context.Context, identity.Selector) (flowupdate.Record, error) {
	return r.item, nil
}

type updateUpdater struct {
	calls   int
	request flowupdate.UpdateRequest
}

func (u *updateUpdater) UpdateFlow(_ context.Context, r flowupdate.UpdateRequest) (flowupdate.UpdateResult, error) {
	u.calls++
	u.request = r
	return flowupdate.UpdateResult{Status: "succeeded", FlowLUID: r.LUID, FlowName: "Prep", ProjectLUID: "p-1", OwnerLUID: *r.OwnerLUID}, nil
}
func TestUpdateUpdateOwnerPreviewsThenRevalidates(t *testing.T) {
	owner := "u-2"
	r := updateResolver{item: flowupdate.Record{LUID: "f-1", Name: "Prep", ProjectLUID: "p-1", OwnerLUID: "u-1"}}
	u := &updateUpdater{}
	in := flowupdate.UpdateInput{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "f-1"}, OwnerLUID: &owner}
	if out, err := flowupdate.Update(context.Background(), r, u, in, true); err != nil || out.Result != nil || u.calls != 0 {
		t.Fatalf("preview=%#v err=%v", out, err)
	}
	if out, err := flowupdate.Update(context.Background(), r, u, in, false); err != nil || out.Result == nil || u.calls != 1 {
		t.Fatalf("execute=%#v err=%v", out, err)
	}
}
func TestUpdateUpdateRequiresOwnerLUID(t *testing.T) {
	if _, err := flowupdate.Update(context.Background(), updateResolver{}, &updateUpdater{}, flowupdate.UpdateInput{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "f-1"}}, false); err == nil {
		t.Fatal("expected owner validation error")
	}
}
