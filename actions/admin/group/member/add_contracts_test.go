package member_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	add "github.com/ahillspace/tadx/actions/admin/group/member"
)

type addContractadapter struct {
	group         add.Group
	result        add.Result
	reads, writes int
}

func (a *addContractadapter) ResolveGroup(context.Context, string) (add.Group, error) {
	a.reads++
	return a.group, nil
}
func (a *addContractadapter) AddGroupUser(context.Context, string, string) (add.Result, error) {
	a.writes++
	return a.result, nil
}
func TestAddObservationAndReceiptContracts(t *testing.T) {
	a := &addContractadapter{group: add.Group{LUID: "group-1", Name: "Authors", Members: nil}, result: add.Result{Status: "added", GroupLUID: "group-1", UserLUID: "user-1"}}
	out, err := add.Add(t.Context(), a, a, add.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil || out.Result.Status != "added" || a.reads != 2 || a.writes != 1 {
		t.Fatalf("out=%#v err=%v addAdapter=%#v", out, err, a)
	}
	raw, err := json.Marshal(out.CompactOutput())
	if err != nil || strings.Contains(string(raw), "username") || strings.Contains(string(raw), "tableau_request_id") || !strings.Contains(string(raw), "\"operation\":\"admin.group.member.add\"") || !strings.Contains(string(raw), "\"no_op\":false") {
		t.Fatalf("output=%s err=%v", raw, err)
	}
}

func (a *addContractadapter) ResolveUsername(context.Context, string) (add.Member, error) {
	panic("unexpected username resolution")
}
