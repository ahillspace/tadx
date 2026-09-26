package member_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	remove "github.com/ahillspace/tadx/actions/admin/group/member"
	"github.com/ahillspace/tadx/internal/errs"
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
	for _, tc := range []struct {
		name  string
		group remove.Group
	}{
		{"wrong group", remove.Group{LUID: "wrong", Name: "Authors"}},
		{"empty name", remove.Group{LUID: "group-1", Name: " "}},
		{"empty member", remove.Group{LUID: "group-1", Name: "Authors", Members: []remove.Member{{}}}},
		{"duplicate member", remove.Group{LUID: "group-1", Name: "Authors", Members: []remove.Member{{LUID: "u"}, {LUID: "u"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &removeContractadapter{group: tc.group}
			_, err := remove.NewRemove(a, a).Execute(t.Context(), remove.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
			detail, ok := errors.AsType[*errs.Error](err)
			if !ok || detail.ID != "admin.group.member.remove.runtime" || a.reads != 1 || a.writes != 0 {
				t.Fatalf("err=%v removeAdapter=%#v", err, a)
			}
		})
	}
	for _, result := range []remove.Result{{GroupLUID: "wrong"}, {UserLUID: "wrong"}, {Status: "unknown"}} {
		a := &removeContractadapter{group: remove.Group{LUID: "group-1", Name: "Authors", Members: []remove.Member{{LUID: "user-1"}}}, result: result}
		_, err := remove.NewRemove(a, a).Execute(t.Context(), remove.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
		detail, ok := errors.AsType[*errs.Error](err)
		if !ok || detail.ID != "admin.group.member.remove.runtime" || a.reads != 2 || a.writes != 1 {
			t.Fatalf("err=%v removeAdapter=%#v", err, a)
		}
	}
	a := &removeContractadapter{group: remove.Group{LUID: "group-1", Name: "Authors", Members: []remove.Member{{LUID: "user-1"}}}, result: remove.Result{Status: "deleted"}}
	out, err := remove.NewRemove(a, a).Execute(t.Context(), remove.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil || out.Result.Status != "deleted" || a.reads != 2 || a.writes != 1 {
		t.Fatalf("out=%#v err=%v removeAdapter=%#v", out, err, a)
	}
	raw, err := json.Marshal(out.CompactOutput())
	if err != nil || strings.Contains(string(raw), "username") || strings.Contains(string(raw), "tableau_request_id") || !strings.Contains(string(raw), "\"operation\":\"admin.group.member.remove\"") || !strings.Contains(string(raw), "\"no_op\":false") {
		t.Fatalf("output=%s err=%v", raw, err)
	}
}
