package group_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	add "github.com/ahillspace/tadx/actions/admin/group"
)

type addContractadapter struct {
	group         add.MembershipGroup
	result        add.MembershipResult
	reads, writes int
}

func (a *addContractadapter) ResolveMembershipGroup(context.Context, string) (add.MembershipGroup, error) {
	a.reads++
	return a.group, nil
}
func (a *addContractadapter) AddMembership(context.Context, string, string) (add.MembershipResult, error) {
	a.writes++
	return a.result, nil
}
func TestAddObservationAndReceiptContracts(t *testing.T) {
	a := &addContractadapter{group: add.MembershipGroup{LUID: "group-1", Name: "Authors", Members: nil}, result: add.MembershipResult{Status: "added", GroupLUID: "group-1", UserLUID: "user-1"}}
	out, err := add.AddMember(t.Context(), a, a, add.MembershipInput{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil || out.Result.Status != "added" || a.reads != 2 || a.writes != 1 {
		t.Fatalf("out=%#v err=%v addAdapter=%#v", out, err, a)
	}
	raw, err := json.Marshal(out.CompactOutput())
	if err != nil || strings.Contains(string(raw), "username") || strings.Contains(string(raw), "tableau_request_id") || !strings.Contains(string(raw), "\"operation\":\"admin.group.member.add\"") || !strings.Contains(string(raw), "\"no_op\":false") {
		t.Fatalf("output=%s err=%v", raw, err)
	}
}

func (a *addContractadapter) ResolveUsername(context.Context, string) (add.MembershipMember, error) {
	panic("unexpected username resolution")
}
