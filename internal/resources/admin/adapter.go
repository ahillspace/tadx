// Package admin isolates administration identity, pagination, and membership rules.
package admin

import (
	"cmp"
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/paging"
	tableau "github.com/ahillspace/tadx/internal/tableau/admin"
)

const (
	resolutionPageSize = tableau.MaxPageSize
	maxResolutionPages = 1000
)

type Adapter struct {
	client          tableau.ClientContract
	checkCapability func(string) error
}

func NewAdapter(client tableau.ClientContract, checks ...func(string) error) *Adapter {
	a := &Adapter{client: client}
	if len(checks) > 0 {
		a.checkCapability = checks[0]
	}
	return a
}

func (a *Adapter) authorize(id string) error {
	if a.checkCapability != nil {
		return a.checkCapability(id)
	}
	return nil
}

// UserSelector accepts an authoritative user LUID or an exact Tableau username.
// Display names and notification email addresses are intentionally not selectors.
type UserSelector struct{ LUID, Username string }
type GroupSelector struct{ LUID, Name string }
type GroupDetail struct {
	Group   tableau.Group
	Members []tableau.User
}

func (a *Adapter) ListUsers(ctx context.Context, input tableau.ListUsersRequest) (tableau.UserPage, error) {
	if err := a.authorize("admin.user.list"); err != nil {
		return tableau.UserPage{}, err
	}
	page, err := a.client.ListUsers(ctx, input)
	if err != nil {
		return tableau.UserPage{}, err
	}
	if err := mergeUsers(make(map[string]tableau.User), page.Items); err != nil {
		return tableau.UserPage{}, err
	}
	return page, nil
}

func (a *Adapter) ResolveUser(ctx context.Context, selector UserSelector) (tableau.User, error) {
	if err := a.authorize("admin.user.inspect"); err != nil {
		return tableau.User{}, err
	}
	if selector.LUID != "" {
		return a.client.GetUser(ctx, selector.LUID)
	}
	if strings.TrimSpace(selector.Username) == "" {
		return tableau.User{}, &identity.ResolutionError{Kind: identity.ResolutionInvalidSelector}
	}
	name := selector.Username
	if _, filterErr := tableau.UserListFilter(tableau.ListUsersRequest{Name: name}); filterErr != nil {
		name = ""
	}
	items, err := a.userInventory(ctx, name)
	if err != nil {
		return tableau.User{}, err
	}
	candidates := make([]identity.Candidate, 0)
	byLUID := make(map[identity.LUID]tableau.User)
	for _, item := range items {
		if item.Name == selector.Username {
			candidate := identity.Candidate{LUID: identity.LUID(item.LUID), Name: selector.Username}
			candidates = append(candidates, candidate)
			byLUID[candidate.LUID] = item
		}
	}
	resolved, err := identity.Resolve(identity.Selector{Name: selector.Username}, candidates)
	if err != nil {
		return tableau.User{}, err
	}
	return byLUID[resolved.LUID], nil
}

func (a *Adapter) UserExists(ctx context.Context, exactNameOrEmail string) (bool, error) {
	if err := a.authorize("admin.user.inspect"); err != nil {
		return false, err
	}
	items, err := a.userInventory(ctx, "")
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.Name == exactNameOrEmail || item.Email == exactNameOrEmail {
			return true, nil
		}
	}
	return false, nil
}

func (a *Adapter) ListGroups(ctx context.Context, input tableau.ListGroupsRequest) (tableau.GroupPage, error) {
	if err := a.authorize("admin.group.list"); err != nil {
		return tableau.GroupPage{}, err
	}
	page, err := a.client.ListGroups(ctx, input)
	if err != nil {
		return tableau.GroupPage{}, err
	}
	if err := mergeGroups(make(map[string]tableau.Group), page.Items); err != nil {
		return tableau.GroupPage{}, err
	}
	return page, nil
}

func (a *Adapter) ResolveGroup(ctx context.Context, selector GroupSelector, includeMembers bool) (GroupDetail, error) {
	if err := a.authorize("admin.group.inspect"); err != nil {
		return GroupDetail{}, err
	}
	items, err := a.allGroups(ctx)
	if err != nil {
		return GroupDetail{}, err
	}
	candidates := make([]identity.Candidate, len(items))
	byLUID := make(map[identity.LUID]tableau.Group, len(items))
	for i, item := range items {
		candidates[i] = identity.Candidate{LUID: identity.LUID(item.LUID), Name: item.Name}
		byLUID[candidates[i].LUID] = item
	}
	resolved, err := identity.Resolve(identity.Selector{LUID: identity.LUID(selector.LUID), Name: selector.Name}, candidates)
	if err != nil {
		return GroupDetail{}, err
	}
	detail := GroupDetail{Group: byLUID[resolved.LUID]}
	if includeMembers {
		detail.Members, err = a.allMembers(ctx, detail.Group.LUID)
		if err != nil {
			return GroupDetail{}, err
		}
	}
	return detail, nil
}

func (a *Adapter) GroupExists(ctx context.Context, exactName string) (bool, error) {
	if err := a.authorize("admin.group.inspect"); err != nil {
		return false, err
	}
	items, err := a.allGroups(ctx)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.Name == exactName {
			return true, nil
		}
	}
	return false, nil
}

func (a *Adapter) GetPermissions(ctx context.Context, input tableau.PermissionRequest) (tableau.PermissionSet, error) {
	if err := a.authorize("admin.permission.inspect"); err != nil {
		return tableau.PermissionSet{}, err
	}
	return a.client.GetPermissions(ctx, input)
}

func (a *Adapter) CreateUser(ctx context.Context, input tableau.CreateUserRequest) (tableau.User, error) {
	if err := a.authorize("admin.user.create"); err != nil {
		return tableau.User{}, err
	}
	return a.client.CreateUser(ctx, input)
}
func (a *Adapter) UpdateUser(ctx context.Context, luid string, input tableau.UpdateUserRequest) (tableau.User, error) {
	if err := a.authorize("admin.user.update"); err != nil {
		return tableau.User{}, err
	}
	return a.client.UpdateUser(ctx, luid, input)
}
func (a *Adapter) DeleteUser(ctx context.Context, luid string) (tableau.MutationResult, error) {
	if err := a.authorize("admin.user.delete"); err != nil {
		return tableau.MutationResult{}, err
	}
	return a.client.DeleteUser(ctx, luid)
}
func (a *Adapter) CreateGroup(ctx context.Context, input tableau.CreateGroupRequest) (tableau.Group, error) {
	if err := a.authorize("admin.group.create"); err != nil {
		return tableau.Group{}, err
	}
	return a.client.CreateGroup(ctx, input)
}
func (a *Adapter) UpdateGroup(ctx context.Context, luid string, input tableau.UpdateGroupRequest) (tableau.Group, error) {
	if err := a.authorize("admin.group.update"); err != nil {
		return tableau.Group{}, err
	}
	return a.client.UpdateGroup(ctx, luid, input)
}
func (a *Adapter) DeleteGroup(ctx context.Context, luid string) (tableau.MutationResult, error) {
	if err := a.authorize("admin.group.delete"); err != nil {
		return tableau.MutationResult{}, err
	}
	return a.client.DeleteGroup(ctx, luid)
}
func (a *Adapter) AddGroupUser(ctx context.Context, groupLUID, userLUID string) (tableau.MutationResult, error) {
	if err := a.authorize("admin.group.member.add"); err != nil {
		return tableau.MutationResult{}, err
	}
	return a.client.AddGroupUser(ctx, groupLUID, userLUID)
}
func (a *Adapter) RemoveGroupUser(ctx context.Context, groupLUID, userLUID string) (tableau.MutationResult, error) {
	if err := a.authorize("admin.group.member.remove"); err != nil {
		return tableau.MutationResult{}, err
	}
	return a.client.RemoveGroupUser(ctx, groupLUID, userLUID)
}

func (a *Adapter) userInventory(ctx context.Context, name string) ([]tableau.User, error) {
	items, err := paging.CollectBounded(ctx, resolutionPageSize, maxResolutionPages, func(ctx context.Context, state paging.State) (paging.Page[tableau.User], error) {
		page, err := a.client.ListUsers(ctx, tableau.ListUsersRequest{PageNumber: state.Number, PageSize: state.Size, Name: name})
		return paging.Page[tableau.User]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Items}, err
	}, func(item tableau.User) string { return item.LUID })
	slices.SortFunc(items, func(a, b tableau.User) int { return cmp.Compare(a.LUID, b.LUID) })
	return items, err
}

func (a *Adapter) allGroups(ctx context.Context) ([]tableau.Group, error) {
	items, err := paging.CollectBounded(ctx, resolutionPageSize, maxResolutionPages, func(ctx context.Context, state paging.State) (paging.Page[tableau.Group], error) {
		page, err := a.client.ListGroups(ctx, tableau.ListGroupsRequest{PageNumber: state.Number, PageSize: state.Size})
		return paging.Page[tableau.Group]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Items}, err
	}, func(item tableau.Group) string { return item.LUID })
	slices.SortFunc(items, func(a, b tableau.Group) int { return cmp.Compare(a.LUID, b.LUID) })
	return items, err
}

func (a *Adapter) allMembers(ctx context.Context, groupLUID string) ([]tableau.User, error) {
	items, err := paging.CollectBounded(ctx, resolutionPageSize, maxResolutionPages, func(ctx context.Context, state paging.State) (paging.Page[tableau.User], error) {
		page, err := a.client.ListGroupUsers(ctx, groupLUID, tableau.PageRequest{PageNumber: state.Number, PageSize: state.Size})
		return paging.Page[tableau.User]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Items}, err
	}, func(item tableau.User) string { return item.LUID })
	slices.SortFunc(items, func(a, b tableau.User) int { return cmp.Compare(a.LUID, b.LUID) })
	return items, err
}

func mergeUsers(target map[string]tableau.User, items []tableau.User) error {
	for _, item := range items {
		if current, ok := target[item.LUID]; ok && !reflect.DeepEqual(current, item) {
			return fmt.Errorf("user inventory returned conflicting records for LUID %q", item.LUID)
		}
		target[item.LUID] = item
	}
	return nil
}
func mergeGroups(target map[string]tableau.Group, items []tableau.Group) error {
	for _, item := range items {
		if current, ok := target[item.LUID]; ok && !reflect.DeepEqual(current, item) {
			return fmt.Errorf("group inventory returned conflicting records for LUID %q", item.LUID)
		}
		target[item.LUID] = item
	}
	return nil
}
