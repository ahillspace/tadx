package member_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	remove "github.com/ahillspace/tadx/actions/admin/group/member"
)

type removeContractadapter struct {
	group         remove.Group
	result        remove.Result
	reads, writes int
}

func (a *removeContractadapter) ResolveGroup(context.Context, string) (remove.Group, error) {
	a.reads++
	return a.group, nil
}
func (a *removeContractadapter) RemoveGroupUser(context.Context, string, string) (remove.Result, error) {
	a.writes++
	return a.result, nil
}
func TestRemoveObservationAndReceiptContracts(t *testing.T) {
	a := &removeContractadapter{group: remove.Group{LUID: "group-1", Name: "Authors", Members: []remove.Member{{LUID: "user-1"}}}, result: remove.Result{Status: "deleted", GroupLUID: "group-1", UserLUID: "user-1"}}
	out, err := remove.Remove(t.Context(), a, a, remove.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil || out.Result.Status != "deleted" || a.reads != 2 || a.writes != 1 {
		t.Fatalf("out=%#v err=%v removeAdapter=%#v", out, err, a)
	}
	raw, err := json.Marshal(out.CompactOutput())
	if err != nil || strings.Contains(string(raw), "username") || strings.Contains(string(raw), "tableau_request_id") || !strings.Contains(string(raw), "\"operation\":\"admin.group.member.remove\"") || !strings.Contains(string(raw), "\"no_op\":false") {
		t.Fatalf("output=%s err=%v", raw, err)
	}
}

func (a *removeContractadapter) ResolveUsername(context.Context, string) (remove.Member, error) {
	panic("unexpected username resolution")
}
