package admin_test

import (
	"context"
	"strings"
	"testing"

	resource "github.com/ahillspace/tadx/internal/resources/admin"
	tableau "github.com/ahillspace/tadx/internal/tableau/admin"
)

type fakeClient struct {
	users       []tableau.User
	groups      []tableau.Group
	members     []tableau.User
	userPages   int
	groupPages  int
	memberPages int
}

func (f *fakeClient) ListUsers(_ context.Context, in tableau.ListUsersRequest) (tableau.UserPage, error) {
	f.userPages++
	return userPage(f.users, in.PageNumber, in.PageSize), nil
}
func (f *fakeClient) GetUser(_ context.Context, luid string) (tableau.User, error) {
	for _, item := range f.users {
		if item.LUID == luid {
			return item, nil
		}
	}
	return tableau.User{}, nil
}
func (f *fakeClient) ListGroups(_ context.Context, in tableau.ListGroupsRequest) (tableau.GroupPage, error) {
	f.groupPages++
	page := groupPage(f.groups, in.PageNumber, in.PageSize)
	return page, nil
}
func (f *fakeClient) ListGroupUsers(_ context.Context, _ string, in tableau.PageRequest) (tableau.UserPage, error) {
	f.memberPages++
	return userPage(f.members, in.PageNumber, in.PageSize), nil
}
func (f *fakeClient) CreateUser(context.Context, tableau.CreateUserRequest) (tableau.User, error) {
	return tableau.User{}, nil
}
func (f *fakeClient) UpdateUser(context.Context, string, tableau.UpdateUserRequest) (tableau.User, error) {
	return tableau.User{}, nil
}
func (f *fakeClient) DeleteUser(context.Context, string) (tableau.MutationResult, error) {
	return tableau.MutationResult{}, nil
}
func (f *fakeClient) CreateGroup(context.Context, tableau.CreateGroupRequest) (tableau.Group, error) {
	return tableau.Group{}, nil
}
func (f *fakeClient) UpdateGroup(context.Context, string, tableau.UpdateGroupRequest) (tableau.Group, error) {
	return tableau.Group{}, nil
}
func (f *fakeClient) DeleteGroup(context.Context, string) (tableau.MutationResult, error) {
	return tableau.MutationResult{}, nil
}
func (f *fakeClient) AddGroupUser(context.Context, string, string) (tableau.MutationResult, error) {
	return tableau.MutationResult{}, nil
}
func (f *fakeClient) RemoveGroupUser(context.Context, string, string) (tableau.MutationResult, error) {
	return tableau.MutationResult{}, nil
}
func (f *fakeClient) GetPermissions(context.Context, tableau.PermissionRequest) (tableau.PermissionSet, error) {
	return tableau.PermissionSet{}, nil
}

func TestAdapterResolvesExactUsersAndRejectsAmbiguity(t *testing.T) {
	client := &fakeClient{users: []tableau.User{{LUID: "u1", Name: "alex", Email: "shared@example.com"}, {LUID: "u2", Name: "jules", Email: "shared@example.com"}}}
	adapter := resource.NewAdapter(client)
	user, err := adapter.ResolveUser(context.Background(), resource.UserSelector{LUID: "u1"})
	if err != nil || user.LUID != "u1" {
		t.Fatalf("ResolveUser(LUID) = %#v, %v", user, err)
	}
	_, err = adapter.ResolveUser(context.Background(), resource.UserSelector{Username: "shared@example.com"})
	if err == nil || !strings.Contains(err.Error(), "no resource matches") {
		t.Fatalf("ResolveUser(non-username) error = %v", err)
	}
}

func TestAdapterResolvesGroupsAndAllDirectMembers(t *testing.T) {
	client := &fakeClient{groups: []tableau.Group{{LUID: "g1", Name: "Authors"}}, members: []tableau.User{{LUID: "u2", Name: "b"}, {LUID: "u1", Name: "a"}}}
	adapter := resource.NewAdapter(client)
	detail, err := adapter.ResolveGroup(context.Background(), resource.GroupSelector{Name: "Authors"}, true)
	if err != nil || detail.Group.LUID != "g1" || len(detail.Members) != 2 || detail.Members[0].LUID != "u1" {
		t.Fatalf("ResolveGroup() = %#v, %v", detail, err)
	}
	if client.groupPages != 1 || client.memberPages != 1 {
		t.Fatalf("pages = group %d, member %d", client.groupPages, client.memberPages)
	}
}

func userPage(items []tableau.User, number, size int) tableau.UserPage {
	start := (number - 1) * size
	if start > len(items) {
		start = len(items)
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return tableau.UserPage{Number: number, Size: size, Total: len(items), Items: append([]tableau.User(nil), items[start:end]...)}
}
func groupPage(items []tableau.Group, number, size int) tableau.GroupPage {
	start := (number - 1) * size
	if start > len(items) {
		start = len(items)
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return tableau.GroupPage{Number: number, Size: size, Total: len(items), Items: append([]tableau.Group(nil), items[start:end]...)}
}
