package admin_test

import (
	"context"
	"fmt"
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
	userFilters []string
	filterUsers bool
}

func (f *fakeClient) ListUsers(_ context.Context, in tableau.ListUsersRequest) (tableau.UserPage, error) {
	f.userPages++
	f.userFilters = append(f.userFilters, in.Name)
	items := f.users
	if f.filterUsers && in.Name != "" {
		items = nil
		for _, user := range f.users {
			if user.Name == in.Name {
				items = append(items, user)
			}
		}
	}
	return userPage(items, in.PageNumber, in.PageSize), nil
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

func TestAdapterUsernameFilterCoversAllMatchingPagesAndFallsBackForUnsafeValues(t *testing.T) {
	users := make([]tableau.User, 1001)
	for index := range users {
		users[index] = tableau.User{LUID: fmt.Sprintf("u-%04d", index), Name: "duplicate"}
	}
	client := &fakeClient{users: users, filterUsers: true}
	adapter := resource.NewAdapter(client)
	if _, err := adapter.ResolveUser(t.Context(), resource.UserSelector{Username: "duplicate"}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatal("duplicate username was not rejected as ambiguous")
	}
	if client.userPages != 2 || fmt.Sprint(client.userFilters) != "[duplicate duplicate]" {
		t.Fatalf("matching pages=%d filters=%v", client.userPages, client.userFilters)
	}
	client.users = []tableau.User{{LUID: "u-sensitive", Name: "user,&"}, {LUID: "u-other", Name: "other", Email: "user,&"}}
	client.userPages, client.userFilters = 0, nil
	user, err := adapter.ResolveUser(t.Context(), resource.UserSelector{Username: "user,&"})
	if err != nil || user.LUID != "u-sensitive" || client.userPages != 1 || fmt.Sprint(client.userFilters) != "[]" {
		t.Fatalf("filter-sensitive resolution = %#v, error=%v, pages=%d, filters=%v", user, err, client.userPages, client.userFilters)
	}
	collisions, err := adapter.FindUsers(t.Context(), "user,&")
	if err != nil || len(collisions) != 2 || client.userFilters[len(client.userFilters)-1] != "" {
		t.Fatalf("name/email collision check = %#v, error=%v, filters=%v", collisions, err, client.userFilters)
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
