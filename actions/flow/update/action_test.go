package update_test

import (
	"context"
	flowupdate "github.com/ahillspace/tadx/actions/flow/update"
	"github.com/ahillspace/tadx/internal/identity"
	"testing"
)

type resolver struct{ item flowupdate.Flow }

func (r resolver) ResolveFlow(context.Context, identity.Selector) (flowupdate.Flow, error) {
	return r.item, nil
}

type updater struct {
	calls   int
	request flowupdate.Request
}

func (u *updater) UpdateFlow(_ context.Context, r flowupdate.Request) (flowupdate.Result, error) {
	u.calls++
	u.request = r
	return flowupdate.Result{Status: "succeeded", FlowLUID: r.LUID, FlowName: "Prep", ProjectLUID: "p-1", OwnerLUID: *r.OwnerLUID}, nil
}
func TestUpdateOwnerPreviewsThenRevalidates(t *testing.T) {
	owner := "u-2"
	r := resolver{item: flowupdate.Flow{LUID: "f-1", Name: "Prep", ProjectLUID: "p-1", OwnerLUID: "u-1"}}
	u := &updater{}
	a := flowupdate.New(r, u)
	in := flowupdate.Input{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "f-1"}, OwnerLUID: &owner}
	if out, err := a.Execute(context.Background(), in, true); err != nil || out.Result != nil || u.calls != 0 {
		t.Fatalf("preview=%#v err=%v", out, err)
	}
	if out, err := a.Execute(context.Background(), in, false); err != nil || out.Result == nil || u.calls != 1 {
		t.Fatalf("execute=%#v err=%v", out, err)
	}
}
func TestUpdateRequiresOwnerLUID(t *testing.T) {
	if _, err := flowupdate.New(resolver{}, &updater{}).Execute(context.Background(), flowupdate.Input{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "f-1"}}, false); err == nil {
		t.Fatal("expected owner validation error")
	}
}
