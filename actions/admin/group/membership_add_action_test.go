package group_test

import (
	"context"
	"testing"

	add "github.com/ahillspace/tadx/actions/admin/group"
)

type addAdapter struct {
	members       []add.MembershipMember
	writes        int
	reads         int
	drift         bool
	usernameReads int
	writtenUser   string
}

func TestAddPreviewDoesNotAddMember(t *testing.T) {
	a := &addAdapter{members: []add.MembershipMember{{LUID: "other"}}}
	out, err := add.AddMember(context.Background(), a, a, add.MembershipInput{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, true)
	if err != nil || out.Plan.Mode != "preview" || out.Plan.NoOp || out.Result != nil || a.writes != 0 {
		t.Fatalf("preview=%#v err=%v writes=%d", out, err, a.writes)
	}
}

func (a *addAdapter) ResolveMembershipGroup(context.Context, string) (add.MembershipGroup, error) {
	a.reads++
	if a.drift && a.reads == 2 {
		a.members = []add.MembershipMember{{LUID: "other"}}
	}
	return add.MembershipGroup{LUID: "group-1", Name: "Authors", Members: append([]add.MembershipMember(nil), a.members...)}, nil
}

func TestAddExecuteRevalidatesPlannedNoOpAfterMembershipDrift(t *testing.T) {
	a := &addAdapter{members: []add.MembershipMember{{LUID: "other"}, {LUID: "user-1"}}, drift: true}
	out, err := add.AddMember(context.Background(), a, a, add.MembershipInput{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Plan.NoOp || a.reads != 2 || a.writes != 1 || out.Result.Status != "added" {
		t.Fatalf("out=%#v reads=%d writes=%d", out, a.reads, a.writes)
	}
}
func (a *addAdapter) AddMembership(_ context.Context, _ string, user string) (add.MembershipResult, error) {
	a.writes++
	a.writtenUser = user
	return add.MembershipResult{Status: "added", GroupLUID: "group-1", UserLUID: user}, nil
}

func (a *addAdapter) ResolveUsername(_ context.Context, username string) (add.MembershipMember, error) {
	a.usernameReads++
	return add.MembershipMember{LUID: "user-1", Name: username}, nil
}

func TestAddUsernameResolutionUsesReturnedIdentityAndReceiptWithoutPostRead(t *testing.T) {
	a := &addAdapter{members: []add.MembershipMember{{LUID: "other"}}}
	out, err := add.AddMember(context.Background(), a, a, add.MembershipInput{Environment: "dev", GroupLUID: "group-1", Username: "analyst@example.test"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.usernameReads != 1 || a.reads != 2 || a.writes != 1 || a.writtenUser != "user-1" || out.Result.Membership != "present" || out.Result.Evidence != "mutation_response" || out.Plan.Username != "analyst@example.test" {
		t.Fatalf("incorrect identity or evidence: out=%#v addAdapter=%#v", out, a)
	}
}

func TestAddMembershipRejectsAmbiguousUserSelectorsBeforeReads(t *testing.T) {
	a := &addAdapter{}
	err := add.ValidateAddMemberInput(add.MembershipInput{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1", Username: "analyst@example.test"})
	if err == nil || a.reads != 0 || a.usernameReads != 0 || a.writes != 0 {
		t.Fatalf("conflicting selectors were resolved: %v %#v", err, a)
	}
}
func TestAddAddIsIdempotentAndPreservesUnrelatedMembers(t *testing.T) {
	a := &addAdapter{members: []add.MembershipMember{{LUID: "other"}}}
	out, err := add.AddMember(context.Background(), a, a, add.MembershipInput{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.writes != 1 || out.Result.Status != "added" {
		t.Fatalf("out=%#v writes=%d", out, a.writes)
	}
	a.members = append(a.members, add.MembershipMember{LUID: "user-1"})
	out, err = add.AddMember(context.Background(), a, a, add.MembershipInput{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.writes != 1 || out.Result.Status != "unchanged" || out.Result.Membership != "present" || out.Result.Evidence != "prewrite_read" {
		t.Fatalf("out=%#v writes=%d", out, a.writes)
	}
}
