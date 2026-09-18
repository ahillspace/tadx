package admin_test

import (
	"errors"
	"testing"

	resource "github.com/ahillspace/tadx/internal/resources/admin"
	tableau "github.com/ahillspace/tadx/internal/tableau/admin"
)

func TestManagedReadChecksPrecedeAdminRequests(t *testing.T) {
	denied := errors.New("managed capability denied")
	client := &fakeClient{}
	adapter := resource.NewAdapter(client, func(string) error { return denied })
	_, userErr := adapter.ResolveUser(t.Context(), resource.UserSelector{Username: "alice"})
	_, groupErr := adapter.ResolveGroup(t.Context(), resource.GroupSelector{Name: "authors"}, true)
	_, listErr := adapter.ListUsers(t.Context(), tableau.ListUsersRequest{PageNumber: 1, PageSize: 100})
	if !errors.Is(userErr, denied) || !errors.Is(groupErr, denied) || !errors.Is(listErr, denied) {
		t.Fatalf("errors: %v %v %v", userErr, groupErr, listErr)
	}
	if client.userPages != 0 || client.groupPages != 0 || client.memberPages != 0 {
		t.Fatal("denied read contacted admin client")
	}
}

func TestManagedInspectDoesNotRequireListCapability(t *testing.T) {
	client := &fakeClient{users: []tableau.User{{LUID: "u1", Name: "alice"}}}
	adapter := resource.NewAdapter(client, func(id string) error {
		if id != "admin.user.inspect" {
			return errors.New("unexpected capability: " + id)
		}
		return nil
	})
	user, err := adapter.ResolveUser(t.Context(), resource.UserSelector{Username: "alice"})
	if err != nil || user.LUID != "u1" || client.userPages != 1 {
		t.Fatalf("user=%#v pages=%d error=%v", user, client.userPages, err)
	}
}
