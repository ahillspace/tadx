package member_test

import (
	"context"
	add "github.com/ahillspace/tadx/actions/admin/group/member"
	"testing"
)

type addAdapter struct {
	members       []add.Member
	writes        int
	reads         int
	drift         bool
	usernameReads int
	writtenUser   string
}

func TestAddPreviewDoesNotAddMember(t *testing.T) {
	a := &addAdapter{members: []add.Member{{LUID: "other"}}}
	out, err := add.NewAdd(a, a).Execute(context.Background(), add.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, true)
	if err != nil || out.Plan.Mode != "preview" || out.Plan.NoOp || out.Result != nil || a.writes != 0 {
		t.Fatalf("preview=%#v err=%v writes=%d", out, err, a.writes)
	}
}

func (a *addAdapter) ResolveGroup(context.Context, string) (add.Group, error) {
	a.reads++
	if a.drift && a.reads == 2 {
		a.members = []add.Member{{LUID: "other"}}
	}
	return add.Group{LUID: "group-1", Name: "Authors", Members: append([]add.Member(nil), a.members...)}, nil
}

func TestAddExecuteRevalidatesPlannedNoOpAfterMembershipDrift(t *testing.T) {
	a := &addAdapter{members: []add.Member{{LUID: "other"}, {LUID: "user-1"}}, drift: true}
	out, err := add.NewAdd(a, a).Execute(context.Background(), add.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Plan.NoOp || a.reads != 2 || a.writes != 1 || out.Result.Status != "added" {
		t.Fatalf("out=%#v reads=%d writes=%d", out, a.reads, a.writes)
	}
}
func (a *addAdapter) AddGroupUser(_ context.Context, _ string, user string) (add.Result, error) {
	a.writes++
	a.writtenUser = user
	return add.Result{Status: "added"}, nil
}

func (a *addAdapter) ResolveUsername(_ context.Context, username string) (add.Member, error) {
	a.usernameReads++
	return add.Member{LUID: "user-1", Name: username}, nil
}

func TestAddUsernameResolutionUsesReturnedIdentityAndReceiptWithoutPostRead(t *testing.T) {
	a := &addAdapter{members: []add.Member{{LUID: "other"}}}
	out, err := add.NewAdd(a, a).Execute(context.Background(), add.Input{Environment: "dev", GroupLUID: "group-1", Username: "analyst@example.test"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.usernameReads != 1 || a.reads != 2 || a.writes != 1 || a.writtenUser != "user-1" || out.Result.Membership != "present" || out.Result.Evidence != "mutation_response" || out.Plan.Username != "analyst@example.test" {
		t.Fatalf("incorrect identity or evidence: out=%#v addAdapter=%#v", out, a)
	}
}

func TestAddMembershipRejectsAmbiguousUserSelectorsBeforeReads(t *testing.T) {
	a := &addAdapter{}
	_, err := add.NewAdd(a, a).Execute(context.Background(), add.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1", Username: "analyst@example.test"}, false)
	if err == nil || a.reads != 0 || a.usernameReads != 0 || a.writes != 0 {
		t.Fatalf("conflicting selectors were resolved: %v %#v", err, a)
	}
}
func TestAddAddIsIdempotentAndPreservesUnrelatedMembers(t *testing.T) {
	a := &addAdapter{members: []add.Member{{LUID: "other"}}}
	out, err := add.NewAdd(a, a).Execute(context.Background(), add.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.writes != 1 || out.Result.Status != "added" {
		t.Fatalf("out=%#v writes=%d", out, a.writes)
	}
	a.members = append(a.members, add.Member{LUID: "user-1"})
	out, err = add.NewAdd(a, a).Execute(context.Background(), add.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.writes != 1 || out.Result.Status != "unchanged" || out.Result.Membership != "present" || out.Result.Evidence != "prewrite_read" {
		t.Fatalf("out=%#v writes=%d", out, a.writes)
	}
}
