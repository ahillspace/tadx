// Package admin isolates administration identity, pagination, and membership rules.
package admin

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
	tableau "github.com/ahillspace/tadx/internal/tableau/admin"
)

const (
	resolutionPageSize = tableau.MaxPageSize
	maxResolutionPages = 1000
)

type Client interface {
	tableau.ClientContract
}

type Adapter struct {
	client          Client
	checkCapability func(string) error
}

func NewAdapter(client Client, checks ...func(string) error) *Adapter {
	a := &Adapter{client: client}
	if len(checks) > 0 {
		a.checkCapability = checks[0]
	}
	return a
}

func (a *Adapter) authorize(id string) error {
	if err := a.configured(); err != nil {
		return err
	}
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
	if err := a.configured(); err != nil {
		return tableau.UserPage{}, err
	}
	page, err := a.client.ListUsers(ctx, input)
	if err != nil {
		return tableau.UserPage{}, err
	}
	if err := validatePage(page.Number, page.Size, page.Total, len(page.Items), input.PageNumber, input.PageSize); err != nil {
		return tableau.UserPage{}, err
	}
	if err := validateUsers(page.Items); err != nil {
		return tableau.UserPage{}, err
	}
	return page, nil
}

func (a *Adapter) ResolveUser(ctx context.Context, selector UserSelector) (tableau.User, error) {
	if err := a.authorize("admin.user.inspect"); err != nil {
		return tableau.User{}, err
	}
	if err := a.configured(); err != nil {
		return tableau.User{}, err
	}
	if selector.LUID != "" {
		user, err := a.client.GetUser(ctx, selector.LUID)
		if err != nil {
			return tableau.User{}, err
		}
		if user.LUID != selector.LUID || strings.TrimSpace(user.Name) == "" {
			return tableau.User{}, errors.New("user get returned an inconsistent authoritative identity")
		}
		return user, nil
	}
	if strings.TrimSpace(selector.Username) == "" {
		return tableau.User{}, &identity.ResolutionError{Kind: identity.ResolutionInvalidSelector}
	}
	items, err := a.allUsers(ctx)
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

func (a *Adapter) FindUsers(ctx context.Context, exactNameOrEmail string) ([]tableau.User, error) {
	if err := a.authorize("admin.user.inspect"); err != nil {
		return nil, err
	}
	items, err := a.allUsers(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]tableau.User, 0)
	for _, item := range items {
		if item.Name == exactNameOrEmail || item.Email == exactNameOrEmail {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LUID < result[j].LUID })
	return result, nil
}

func (a *Adapter) ListGroups(ctx context.Context, input tableau.ListGroupsRequest) (tableau.GroupPage, error) {
	if err := a.authorize("admin.group.list"); err != nil {
		return tableau.GroupPage{}, err
	}
	if err := a.configured(); err != nil {
		return tableau.GroupPage{}, err
	}
	page, err := a.client.ListGroups(ctx, input)
	if err != nil {
		return tableau.GroupPage{}, err
	}
	if err := validatePage(page.Number, page.Size, page.Total, len(page.Items), input.PageNumber, input.PageSize); err != nil {
		return tableau.GroupPage{}, err
	}
	if err := validateGroups(page.Items); err != nil {
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

func (a *Adapter) FindGroups(ctx context.Context, exactName string) ([]tableau.Group, error) {
	if err := a.authorize("admin.group.inspect"); err != nil {
		return nil, err
	}
	items, err := a.allGroups(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]tableau.Group, 0)
	for _, item := range items {
		if item.Name == exactName {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LUID < result[j].LUID })
	return result, nil
}

func (a *Adapter) GetPermissions(ctx context.Context, input tableau.PermissionRequest) (tableau.PermissionSet, error) {
	if err := a.authorize("admin.permission.inspect"); err != nil {
		return tableau.PermissionSet{}, err
	}
	if err := a.configured(); err != nil {
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

func (a *Adapter) allUsers(ctx context.Context) ([]tableau.User, error) {
	if err := a.configured(); err != nil {
		return nil, err
	}
	byID := make(map[string]tableau.User)
	for number := 1; number <= maxResolutionPages; number++ {
		page, err := a.client.ListUsers(ctx, tableau.ListUsersRequest{PageNumber: number, PageSize: resolutionPageSize})
		if err != nil {
			return nil, err
		}
		if err := validatePage(page.Number, page.Size, page.Total, len(page.Items), number, resolutionPageSize); err != nil {
			return nil, err
		}
		if err := mergeUsers(byID, page.Items); err != nil {
			return nil, err
		}
		if number*page.Size >= page.Total {
			return sortedUsers(byID), nil
		}
	}
	return nil, errors.New("user inventory exceeded the 1000-page resolution bound")
}

func (a *Adapter) allGroups(ctx context.Context) ([]tableau.Group, error) {
	if err := a.configured(); err != nil {
		return nil, err
	}
	byID := make(map[string]tableau.Group)
	for number := 1; number <= maxResolutionPages; number++ {
		page, err := a.client.ListGroups(ctx, tableau.ListGroupsRequest{PageNumber: number, PageSize: resolutionPageSize})
		if err != nil {
			return nil, err
		}
		if err := validatePage(page.Number, page.Size, page.Total, len(page.Items), number, resolutionPageSize); err != nil {
			return nil, err
		}
		if err := mergeGroups(byID, page.Items); err != nil {
			return nil, err
		}
		if number*page.Size >= page.Total {
			result := make([]tableau.Group, 0, len(byID))
			for _, item := range byID {
				result = append(result, item)
			}
			sort.Slice(result, func(i, j int) bool { return result[i].LUID < result[j].LUID })
			return result, nil
		}
	}
	return nil, errors.New("group inventory exceeded the 1000-page resolution bound")
}

func (a *Adapter) allMembers(ctx context.Context, groupLUID string) ([]tableau.User, error) {
	byID := make(map[string]tableau.User)
	for number := 1; number <= maxResolutionPages; number++ {
		page, err := a.client.ListGroupUsers(ctx, groupLUID, tableau.PageRequest{PageNumber: number, PageSize: resolutionPageSize})
		if err != nil {
			return nil, err
		}
		if err := validatePage(page.Number, page.Size, page.Total, len(page.Items), number, resolutionPageSize); err != nil {
			return nil, err
		}
		if err := mergeUsers(byID, page.Items); err != nil {
			return nil, err
		}
		if number*page.Size >= page.Total {
			return sortedUsers(byID), nil
		}
	}
	return nil, errors.New("group membership exceeded the 1000-page resolution bound")
}

func (a *Adapter) configured() error {
	if a == nil || a.client == nil {
		return errors.New("administration resource adapter is not configured")
	}
	return nil
}
func validatePage(number, size, total, count, expectedNumber, maxSize int) error {
	if number != expectedNumber || size <= 0 || size > maxSize || total < 0 || count > size || (number-1)*size+count > total {
		return fmt.Errorf("administration reader returned inconsistent pagination number=%d size=%d total=%d count=%d", number, size, total, count)
	}
	return nil
}
func validateUsers(items []tableau.User) error {
	return mergeUsers(make(map[string]tableau.User), items)
}
func validateGroups(items []tableau.Group) error {
	return mergeGroups(make(map[string]tableau.Group), items)
}
func mergeUsers(target map[string]tableau.User, items []tableau.User) error {
	for _, item := range items {
		item.LUID, item.Name = strings.TrimSpace(item.LUID), strings.TrimSpace(item.Name)
		if item.LUID == "" || item.Name == "" {
			return errors.New("user inventory omitted authoritative identity")
		}
		if current, ok := target[item.LUID]; ok && !reflect.DeepEqual(current, item) {
			return fmt.Errorf("user inventory returned conflicting records for LUID %q", item.LUID)
		}
		target[item.LUID] = item
	}
	return nil
}
func mergeGroups(target map[string]tableau.Group, items []tableau.Group) error {
	for _, item := range items {
		item.LUID, item.Name = strings.TrimSpace(item.LUID), strings.TrimSpace(item.Name)
		if item.LUID == "" || item.Name == "" {
			return errors.New("group inventory omitted authoritative identity")
		}
		if current, ok := target[item.LUID]; ok && !reflect.DeepEqual(current, item) {
			return fmt.Errorf("group inventory returned conflicting records for LUID %q", item.LUID)
		}
		target[item.LUID] = item
	}
	return nil
}
func sortedUsers(byID map[string]tableau.User) []tableau.User {
	result := make([]tableau.User, 0, len(byID))
	for _, item := range byID {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LUID < result[j].LUID })
	return result
}
