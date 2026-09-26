package member_test

import (
	"context"
	remove "github.com/ahillspace/tadx/actions/admin/group/member"
	"testing"
)

type removeAdapter struct {
	members       []remove.Member
	writes        int
	reads         int
	drift         bool
	usernameReads int
	writtenUser   string
}

func TestRemovePreviewDoesNotRemoveMember(t *testing.T) {
	a := &removeAdapter{members: []remove.Member{{LUID: "user-1"}, {LUID: "other"}}}
	out, err := remove.NewRemove(a, a).Execute(context.Background(), remove.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, true)
	if err != nil || out.Plan.Mode != "preview" || out.Plan.NoOp || out.Result != nil || a.writes != 0 {
		t.Fatalf("preview=%#v err=%v writes=%d", out, err, a.writes)
	}
}

func (a *removeAdapter) ResolveGroup(context.Context, string) (remove.Group, error) {
	a.reads++
	if a.drift && a.reads == 2 {
		a.members = append(a.members, remove.Member{LUID: "user-1"})
	}
	return remove.Group{LUID: "group-1", Name: "Authors", Members: append([]remove.Member(nil), a.members...)}, nil
}

func TestRemoveExecuteRevalidatesPlannedNoOpAfterMembershipDrift(t *testing.T) {
	a := &removeAdapter{members: []remove.Member{{LUID: "other"}}, drift: true}
	out, err := remove.NewRemove(a, a).Execute(context.Background(), remove.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Plan.NoOp || a.reads != 2 || a.writes != 1 || out.Result.Status != "removed" {
		t.Fatalf("out=%#v reads=%d writes=%d", out, a.reads, a.writes)
	}
}
func (a *removeAdapter) RemoveGroupUser(_ context.Context, _ string, user string) (remove.Result, error) {
	a.writes++
	a.writtenUser = user
	return remove.Result{Status: "removed"}, nil
}

func (a *removeAdapter) ResolveUsername(_ context.Context, username string) (remove.Member, error) {
	a.usernameReads++
	return remove.Member{LUID: "user-1", Name: username}, nil
}

func TestRemoveUsernameResolutionUsesReturnedIdentityAndReceiptWithoutPostRead(t *testing.T) {
	a := &removeAdapter{members: []remove.Member{{LUID: "user-1"}, {LUID: "other"}}}
	out, err := remove.NewRemove(a, a).Execute(context.Background(), remove.Input{Environment: "dev", GroupLUID: "group-1", Username: "analyst@example.test"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.usernameReads != 1 || a.reads != 2 || a.writes != 1 || a.writtenUser != "user-1" || out.Result.Membership != "absent" || out.Result.Evidence != "mutation_response" || out.Plan.Username != "analyst@example.test" {
		t.Fatalf("incorrect identity or evidence: out=%#v removeAdapter=%#v", out, a)
	}
}

func TestRemoveMembershipRejectsAmbiguousUserSelectorsBeforeReads(t *testing.T) {
	a := &removeAdapter{}
	_, err := remove.NewRemove(a, a).Execute(context.Background(), remove.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1", Username: "analyst@example.test"}, false)
	if err == nil || a.reads != 0 || a.usernameReads != 0 || a.writes != 0 {
		t.Fatalf("conflicting selectors were resolved: %v %#v", err, a)
	}
}
func TestRemoveRemoveIsIdempotentAndPreservesUnrelatedMembers(t *testing.T) {
	a := &removeAdapter{members: []remove.Member{{LUID: "other"}, {LUID: "user-1"}}}
	out, err := remove.NewRemove(a, a).Execute(context.Background(), remove.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.writes != 1 || out.Result.Status != "removed" {
		t.Fatalf("out=%#v writes=%d", out, a.writes)
	}
	a.members = []remove.Member{{LUID: "other"}}
	out, err = remove.NewRemove(a, a).Execute(context.Background(), remove.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.writes != 1 || out.Result.Status != "unchanged" || out.Result.Membership != "absent" || out.Result.Evidence != "prewrite_read" {
		t.Fatalf("out=%#v writes=%d", out, a.writes)
	}
}
