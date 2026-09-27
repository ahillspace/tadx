package group_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	action "github.com/ahillspace/tadx/actions/admin/group"
	"github.com/ahillspace/tadx/internal/value"
)

type resolver struct{ members []action.Member }

func (r resolver) ResolveGroup(context.Context, action.Selector, bool) (action.Record, error) {
	return action.Record{AdminGroup: value.AdminGroup{LUID: "group-1", Name: "Authors"}, Members: r.members}, nil
}

func TestFullOutputKeepsExplicitlyRequestedMembers(t *testing.T) {
	members := make([]action.Member, 201)
	for i := range members {
		members[i] = action.Member{LUID: "user-" + string(rune(i+1))}
	}
	out, err := action.Inspect(context.Background(), resolver{members: members}, action.InspectInput{Selector: action.Selector{LUID: "group-1"}, IncludeMembers: true})
	if err != nil {
		t.Fatal(err)
	}
	full, ok := out.FullOutput().(action.InspectFullResult)
	if !ok {
		t.Fatalf("FullOutput() type = %T", out.FullOutput())
	}
	if len(full.Group.Members) != len(members) || full.Group.MembersOmitted != 0 {
		t.Fatalf("full members = %d omitted=%d, want %d and 0", len(full.Group.Members), full.Group.MembersOmitted, len(members))
	}
	compact, err := json.Marshal(out.CompactOutput())
	if err != nil || strings.Count(string(compact), `"luid":`) != len(members)+1 {
		t.Fatalf("compact explicit members were lost: %s err=%v", compact, err)
	}
}

func TestEmptyRequestedMembersAndUnreportedSettingRemainExplicit(t *testing.T) {
	out, err := action.Inspect(t.Context(), resolver{}, action.InspectInput{Selector: action.Selector{LUID: "group-1"}, IncludeMembers: true})
	if err != nil {
		t.Fatal(err)
	}
	compact, _ := json.Marshal(out.CompactOutput())
	if !strings.Contains(string(compact), `"members":[]`) {
		t.Fatalf("missing observed empty membership: %s", compact)
	}
	full, _ := json.Marshal(out.FullOutput())
	if !strings.Contains(string(full), `"external_user_enabled_state":"not_reported"`) {
		t.Fatalf("missing omitted-setting evidence: %s", full)
	}
}

func TestFullOutputBoundsUnrequestedMembers(t *testing.T) {
	members := make([]action.Member, 201)
	out, err := action.Inspect(context.Background(), resolver{members: members}, action.InspectInput{Selector: action.Selector{LUID: "group-1"}})
	if err != nil {
		t.Fatal(err)
	}
	full := out.FullOutput().(action.InspectFullResult)
	if len(full.Group.Members) != 100 || full.Group.MembersOmitted != 101 {
		t.Fatalf("full members = %d omitted=%d, want 100 and 101", len(full.Group.Members), full.Group.MembersOmitted)
	}
}
