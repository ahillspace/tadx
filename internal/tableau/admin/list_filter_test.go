package admin_test

import (
	adapter "github.com/ahillspace/tadx/internal/tableau/admin"
	"testing"
)

func TestTypedListFilter(t *testing.T) {
	got, err := adapter.UserListFilter(adapter.ListUsersRequest{Name: "Sales", SiteRole: "Viewer"})
	if err != nil || got != "name:eq:Sales,siteRole:eq:Viewer" {
		t.Fatalf("filter=%q error=%v", got, err)
	}
	if got, err := adapter.UserListFilter(adapter.ListUsersRequest{}); err != nil || got != "" {
		t.Fatalf("empty filter=%q error=%v", got, err)
	}
	for _, value := range []string{"One,Two", "One&Two"} {
		if _, err := adapter.UserListFilter(adapter.ListUsersRequest{Name: value}); err == nil {
			t.Fatalf("accepted delimiter %q", value)
		}
	}
}

func TestTypedGroupListFilter(t *testing.T) {
	got, err := adapter.GroupListFilter(adapter.ListGroupsRequest{Name: "Team", Domain: "local"})
	if err != nil || got != "name:eq:Team,domainName:eq:local" {
		t.Fatalf("filter=%q error=%v", got, err)
	}
	if got, err := adapter.GroupListFilter(adapter.ListGroupsRequest{}); err != nil || got != "" {
		t.Fatalf("empty filter=%q error=%v", got, err)
	}
	for _, input := range []adapter.ListGroupsRequest{{Name: "One,Two"}, {Domain: "One&Two"}} {
		if _, err := adapter.GroupListFilter(input); err == nil {
			t.Fatalf("accepted delimiter: %+v", input)
		}
	}
}
