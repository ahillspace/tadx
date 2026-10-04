package admin

import (
	"context"

	group "github.com/ahillspace/tadx/actions/admin/group"
	tableau "github.com/ahillspace/tadx/internal/tableau/admin"
)

// GroupPorts translates native administration records into the group action's ports.
type GroupPorts struct{ Adapter *Adapter }

// GroupListFilterPort validates and encodes a native group selection before authentication.
type GroupListFilterPort struct{}

func (GroupListFilterPort) ListFilter(input group.ListInput) (string, error) {
	return tableau.GroupListFilter(tableau.ListGroupsRequest{Name: input.Name, Domain: input.Domain})
}

func (p GroupPorts) ListGroups(ctx context.Context, in group.ListPageRequest) (group.ListPage, error) {
	page, err := p.Adapter.ListGroups(ctx, tableau.ListGroupsRequest{PageNumber: in.PageNumber, PageSize: in.PageSize, Name: in.Name, Domain: in.Domain})
	items := make([]group.Record, len(page.Items))
	for i, item := range page.Items {
		items[i] = groupRecord(item)
	}
	return group.ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Groups: items, RequestID: page.RequestID}, err
}

func groupRecord(item tableau.Group) group.Record {
	return group.Record{LUID: item.LUID, Name: item.Name, Domain: item.Domain, MinimumSiteRole: item.MinimumSiteRole, GrantLicenseMode: item.GrantLicenseMode, ExternalUserEnabled: item.ExternalUserEnabled, RequestID: item.RequestID, MutationStatus: item.MutationStatus}
}

func (p GroupPorts) ResolveGroup(ctx context.Context, selector group.Selector, members bool) (group.Record, error) {
	detail, err := p.Adapter.ResolveGroup(ctx, GroupSelector{LUID: selector.LUID, Name: selector.Name}, members)
	item := groupRecord(detail.Group)
	item.Members = make([]group.Member, len(detail.Members))
	for i, member := range detail.Members {
		item.Members[i] = group.Member{LUID: member.LUID, Name: member.Name, SiteRole: member.SiteRole}
	}
	return item, err
}

func (p GroupPorts) GroupExists(ctx context.Context, name string) (bool, error) {
	return p.Adapter.GroupExists(ctx, name)
}

func (p GroupPorts) CreateGroup(ctx context.Context, in group.CreateRequest) (group.Record, error) {
	item, err := p.Adapter.CreateGroup(ctx, tableau.CreateGroupRequest{Name: in.Name, MinimumSiteRole: in.MinimumSiteRole, ExternalUserEnabled: in.ExternalUserEnabled})
	return groupRecord(item), err
}

func (p GroupPorts) UpdateGroup(ctx context.Context, luid string, in group.UpdateRequest) (group.Record, error) {
	item, err := p.Adapter.UpdateGroup(ctx, luid, tableau.UpdateGroupRequest{Name: in.Name, MinimumSiteRole: in.MinimumSiteRole, ExternalUserEnabled: in.ExternalUserEnabled})
	return groupRecord(item), err
}

func (p GroupPorts) AddGroupUser(ctx context.Context, groupLUID, userLUID string) (string, error) {
	item, err := p.Adapter.AddGroupUser(ctx, groupLUID, userLUID)
	return item.RequestID, err
}

func (p GroupPorts) RemoveGroupUser(ctx context.Context, groupLUID, userLUID string) (string, error) {
	item, err := p.Adapter.RemoveGroupUser(ctx, groupLUID, userLUID)
	return item.RequestID, err
}

func (p GroupPorts) DeleteGroup(ctx context.Context, luid string) (group.DeleteResult, error) {
	item, err := p.Adapter.DeleteGroup(ctx, luid)
	return group.DeleteResult{Status: item.Status, GroupLUID: item.ResourceLUID, TableauRequestID: item.RequestID}, err
}

func (p GroupPorts) ResolveUsername(ctx context.Context, username string) (group.MembershipMember, error) {
	user, err := p.Adapter.ResolveUser(ctx, UserSelector{Username: username})
	return group.MembershipMember{LUID: user.LUID, Name: user.Name}, err
}

func (p GroupPorts) ResolveMembershipGroup(ctx context.Context, luid string) (group.MembershipGroup, error) {
	detail, err := p.Adapter.ResolveGroup(ctx, GroupSelector{LUID: luid}, true)
	members := make([]group.MembershipMember, len(detail.Members))
	for i, member := range detail.Members {
		members[i] = group.MembershipMember{LUID: member.LUID, Name: member.Name}
	}
	return group.MembershipGroup{LUID: detail.Group.LUID, Name: detail.Group.Name, Members: members}, err
}

func (p GroupPorts) AddMembership(ctx context.Context, groupLUID, userLUID string) (group.MembershipResult, error) {
	item, err := p.Adapter.AddGroupUser(ctx, groupLUID, userLUID)
	return group.MembershipResult{Status: item.Status, GroupLUID: groupLUID, UserLUID: item.ResourceLUID, TableauRequestID: item.RequestID}, err
}

func (p GroupPorts) RemoveMembership(ctx context.Context, groupLUID, userLUID string) (group.MembershipResult, error) {
	item, err := p.Adapter.RemoveGroupUser(ctx, groupLUID, userLUID)
	return group.MembershipResult{Status: item.Status, GroupLUID: groupLUID, UserLUID: item.ResourceLUID, TableauRequestID: item.RequestID}, err
}
