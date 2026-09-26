package member_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	add "github.com/ahillspace/tadx/actions/admin/group/member"
	"github.com/ahillspace/tadx/internal/errs"
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
	for _, tc := range []struct {
		name  string
		group add.Group
	}{
		{"wrong group", add.Group{LUID: "wrong", Name: "Authors"}},
		{"empty name", add.Group{LUID: "group-1", Name: " "}},
		{"empty member", add.Group{LUID: "group-1", Name: "Authors", Members: []add.Member{{}}}},
		{"duplicate member", add.Group{LUID: "group-1", Name: "Authors", Members: []add.Member{{LUID: "u"}, {LUID: "u"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &addContractadapter{group: tc.group}
			_, err := add.NewAdd(a, a).Execute(t.Context(), add.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
			detail, ok := errors.AsType[*errs.Error](err)
			if !ok || detail.ID != "admin.group.member.add.runtime" || a.reads != 1 || a.writes != 0 {
				t.Fatalf("err=%v addAdapter=%#v", err, a)
			}
		})
	}
	for _, result := range []add.Result{{GroupLUID: "wrong"}, {UserLUID: "wrong"}, {Status: "unknown"}} {
		a := &addContractadapter{group: add.Group{LUID: "group-1", Name: "Authors", Members: nil}, result: result}
		_, err := add.NewAdd(a, a).Execute(t.Context(), add.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
		detail, ok := errors.AsType[*errs.Error](err)
		if !ok || detail.ID != "admin.group.member.add.runtime" || a.reads != 2 || a.writes != 1 {
			t.Fatalf("err=%v addAdapter=%#v", err, a)
		}
	}
	a := &addContractadapter{group: add.Group{LUID: "group-1", Name: "Authors", Members: nil}, result: add.Result{Status: "added"}}
	out, err := add.NewAdd(a, a).Execute(t.Context(), add.Input{Environment: "dev", GroupLUID: "group-1", UserLUID: "user-1"}, false)
	if err != nil || out.Result.Status != "added" || a.reads != 2 || a.writes != 1 {
		t.Fatalf("out=%#v err=%v addAdapter=%#v", out, err, a)
	}
	raw, err := json.Marshal(out.CompactOutput())
	if err != nil || strings.Contains(string(raw), "username") || strings.Contains(string(raw), "tableau_request_id") || !strings.Contains(string(raw), "\"operation\":\"admin.group.member.add\"") || !strings.Contains(string(raw), "\"no_op\":false") {
		t.Fatalf("output=%s err=%v", raw, err)
	}
}
