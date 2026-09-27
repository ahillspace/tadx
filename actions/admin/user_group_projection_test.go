package admin_test

import (
	"encoding/json"
	"testing"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	userops "github.com/ahillspace/tadx/actions/admin/user"
	"github.com/ahillspace/tadx/internal/value"
)

func TestUserGroupStoredAndListProjectionContracts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"user list", userops.ListUser{LUID: "u", Name: "login"}, `{"luid":"u","name":"login"}`},
		{"user detail", userops.Record{LUID: "u", Name: "login", RequestID: "private"}, `{"luid":"u","name":"login"}`},
		{"group list", groupops.ListGroup{LUID: "g", Name: "Group", ExternalUserEnabled: new(false)}, `{"luid":"g","name":"Group","external_user_enabled":false}`},
		{"group detail without members", groupops.Record{AdminGroup: value.AdminGroup{LUID: "g", Name: "Group", RequestID: "private"}}, `{"luid":"g","name":"Group","members":null}`},
		{"group detail empty members", groupops.Record{AdminGroup: value.AdminGroup{LUID: "g", Name: "Group"}, Members: []groupops.Member{}}, `{"luid":"g","name":"Group","members":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.value)
			if err != nil || string(got) != tc.want {
				t.Fatalf("projection = %s, %v; want %s", got, err, tc.want)
			}
		})
	}
}
