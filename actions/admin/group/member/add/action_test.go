package add_test

import (
	"context"
	add "github.com/ahillspace/tadx/actions/admin/group/member/add"
	"testing"
)

type adapter struct {
	members []add.Member
	writes  int
	reads   int
	drift   bool
}

func TestPreviewDoesNotAddMember(t *testing.T) {
	a := &adapter{members: []add.Member{{LUID: "other"}}}
	out, err := add.New(a, a).Execute(context.Background(), add.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, true)
	if err != nil || out.Plan.Mode != "preview" || out.Plan.NoOp || out.Result != nil || a.writes != 0 {
		t.Fatalf("preview=%#v err=%v writes=%d", out, err, a.writes)
	}
}

func (a *adapter) ResolveGroup(context.Context, string) (add.Group, error) {
	a.reads++
	if a.drift && a.reads == 2 {
		a.members = []add.Member{{LUID: "other"}}
	}
	return add.Group{LUID: "group-1", Name: "Authors", Members: append([]add.Member(nil), a.members...)}, nil
}

func TestExecuteRevalidatesPlannedNoOpAfterMembershipDrift(t *testing.T) {
	a := &adapter{members: []add.Member{{LUID: "other"}, {LUID: "user-1"}}, drift: true}
	out, err := add.New(a, a).Execute(context.Background(), add.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Plan.NoOp || a.reads != 2 || a.writes != 1 || out.Result.Status != "added" {
		t.Fatalf("out=%#v reads=%d writes=%d", out, a.reads, a.writes)
	}
}
func (a *adapter) AddGroupUser(context.Context, string, string) (add.Result, error) {
	a.writes++
	return add.Result{Status: "added"}, nil
}
func TestAddIsIdempotentAndPreservesUnrelatedMembers(t *testing.T) {
	a := &adapter{members: []add.Member{{LUID: "other"}}}
	out, err := add.New(a, a).Execute(context.Background(), add.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.writes != 1 || out.Result.Status != "added" {
		t.Fatalf("out=%#v writes=%d", out, a.writes)
	}
	a.members = append(a.members, add.Member{LUID: "user-1"})
	out, err = add.New(a, a).Execute(context.Background(), add.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.writes != 1 || out.Result.Status != "unchanged" {
		t.Fatalf("out=%#v writes=%d", out, a.writes)
	}
}
