package remove_test

import (
	"context"
	remove "github.com/ahillspace/tadx/actions/admin/group/member/remove"
	"testing"
)

type adapter struct {
	members []remove.Member
	writes  int
	reads   int
	drift   bool
}

func (a *adapter) ResolveGroup(context.Context, string) (remove.Group, error) {
	a.reads++
	if a.drift && a.reads == 2 {
		a.members = append(a.members, remove.Member{LUID: "user-1"})
	}
	return remove.Group{LUID: "group-1", Name: "Authors", Members: append([]remove.Member(nil), a.members...)}, nil
}

func TestExecuteRevalidatesPlannedNoOpAfterMembershipDrift(t *testing.T) {
	a := &adapter{members: []remove.Member{{LUID: "other"}}, drift: true}
	out, err := remove.New(a, a).Execute(context.Background(), remove.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Plan.NoOp || a.reads != 2 || a.writes != 1 || out.Result.Status != "removed" {
		t.Fatalf("out=%#v reads=%d writes=%d", out, a.reads, a.writes)
	}
}
func (a *adapter) RemoveGroupUser(context.Context, string, string) (remove.Result, error) {
	a.writes++
	return remove.Result{Status: "removed"}, nil
}
func TestRemoveIsIdempotentAndPreservesUnrelatedMembers(t *testing.T) {
	a := &adapter{members: []remove.Member{{LUID: "other"}, {LUID: "user-1"}}}
	out, err := remove.New(a, a).Execute(context.Background(), remove.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.writes != 1 || out.Result.Status != "removed" {
		t.Fatalf("out=%#v writes=%d", out, a.writes)
	}
	a.members = []remove.Member{{LUID: "other"}}
	out, err = remove.New(a, a).Execute(context.Background(), remove.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.writes != 1 || out.Result.Status != "unchanged" {
		t.Fatalf("out=%#v writes=%d", out, a.writes)
	}
}
