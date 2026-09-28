package app

import (
	"context"
	"net/url"
	"strings"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	userops "github.com/ahillspace/tadx/actions/admin/user"
	"github.com/ahillspace/tadx/internal/errs"
	resourceadmin "github.com/ahillspace/tadx/internal/resources/admin"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
)

type adminUserAdapter struct {
	*resourceadmin.Adapter
	serverURL  string
	callerLUID string
}

func (a adminUserAdapter) ValidateUpdate(ctx context.Context, input userops.UpdateRequest) error {
	if input.FullName == nil {
		return nil
	}
	server, err := url.Parse(a.serverURL)
	if err != nil {
		return err
	}
	host := strings.ToLower(server.Hostname())
	if host == "online.tableau.com" || strings.HasSuffix(host, ".online.tableau.com") {
		return unsupportedUserFullName("Tableau Cloud does not support updating a user's full name.")
	}
	if a.callerLUID == "" {
		return nil
	}
	caller, err := a.Adapter.ResolveUser(ctx, resourceadmin.UserSelector{LUID: a.callerLUID})
	if err != nil {
		return nil
	}
	if caller.SiteRole == "SiteAdministratorCreator" || caller.SiteRole == "SiteAdministratorExplorer" {
		return unsupportedUserFullName("A site administrator cannot update a user's full name.")
	}
	return nil
}

func unsupportedUserFullName(summary string) error {
	return &errs.Error{ID: "admin.user.update.unsupported_full_name", Kind: errs.KindUsage, Operation: "admin.user.update", Summary: summary, Retryable: errs.Bool(false), CorrectiveAction: "Remove --full-name and review a new preview.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
}

func (a adminUserAdapter) ListUsers(ctx context.Context, input userops.ListPageRequest) (userops.ListPage, error) {
	page, err := a.Adapter.ListUsers(ctx, tableauadmin.ListUsersRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, SiteRole: input.SiteRole})
	items := make([]userops.Record, len(page.Items))
	for i, item := range page.Items {
		items[i] = adminUserRecord(item)
	}
	return userops.ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Users: items, RequestID: page.RequestID}, err
}

func (a adminUserAdapter) ResolveUser(ctx context.Context, selector userops.Selector) (userops.Record, error) {
	item, err := a.Adapter.ResolveUser(ctx, resourceadmin.UserSelector{LUID: selector.LUID, Username: selector.Username})
	return adminUserRecord(item), err
}

func adminUserRecord(item tableauadmin.User) userops.Record {
	return userops.Record{LUID: item.LUID, Name: item.Name, FullName: item.FullName, Email: item.Email, SiteRole: item.SiteRole, LastLogin: item.LastLogin, ExternalAuthUserID: item.ExternalAuthUserID, AuthSetting: item.AuthSetting, IdentityPoolName: item.IdentityPoolName, IdPConfigurationID: item.IdPConfigurationID, Language: item.Language, Locale: item.Locale, Domain: item.Domain, RequestID: item.RequestID, MutationStatus: item.MutationStatus, PresentFields: item.PresentFields}
}

func (a adminUserAdapter) CreateUser(ctx context.Context, input userops.CreateRequest) (userops.Record, error) {
	item, err := a.Adapter.CreateUser(ctx, tableauadmin.CreateUserRequest{Name: input.Name, SiteRole: input.SiteRole, AuthSetting: input.AuthSetting, IdentityPoolName: input.IdentityPoolName, IdPConfigurationID: input.IdPConfigurationID, Email: input.Email, Language: input.Language, Locale: input.Locale})
	return adminUserRecord(item), err
}

func (a adminUserAdapter) UpdateUser(ctx context.Context, luid string, input userops.UpdateRequest) (userops.Record, error) {
	item, err := a.Adapter.UpdateUser(ctx, luid, tableauadmin.UpdateUserRequest{FullName: input.FullName, Email: input.Email, SiteRole: input.SiteRole, AuthSetting: input.AuthSetting, IdentityPoolName: input.IdentityPoolName, IdPConfigurationID: input.IdPConfigurationID, Language: input.Language, Locale: input.Locale})
	return adminUserRecord(item), err
}

func (a adminUserAdapter) DeleteUser(ctx context.Context, luid string) (userops.DeleteResult, error) {
	item, err := a.Adapter.DeleteUser(ctx, luid)
	return userops.DeleteResult{Status: item.Status, UserLUID: item.ResourceLUID, TableauRequestID: item.RequestID}, err
}

type adminGroupAdapter struct{ *resourceadmin.Adapter }

func (a adminGroupAdapter) ListGroups(ctx context.Context, input groupops.ListPageRequest) (groupops.ListPage, error) {
	page, err := a.Adapter.ListGroups(ctx, tableauadmin.ListGroupsRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, Domain: input.Domain})
	items := make([]groupops.Record, len(page.Items))
	for i, item := range page.Items {
		items[i] = adminGroupRecord(item)
	}
	return groupops.ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Groups: items, RequestID: page.RequestID}, err
}

func adminGroupRecord(item tableauadmin.Group) groupops.Record {
	return groupops.Record{LUID: item.LUID, Name: item.Name, Domain: item.Domain, MinimumSiteRole: item.MinimumSiteRole, GrantLicenseMode: item.GrantLicenseMode, ExternalUserEnabled: item.ExternalUserEnabled, RequestID: item.RequestID, MutationStatus: item.MutationStatus}
}

func (a adminGroupAdapter) ResolveGroup(ctx context.Context, selector groupops.Selector, members bool) (groupops.Record, error) {
	detail, err := a.Adapter.ResolveGroup(ctx, resourceadmin.GroupSelector{LUID: selector.LUID, Name: selector.Name}, members)
	item := adminGroupRecord(detail.Group)
	item.Members = make([]groupops.Member, len(detail.Members))
	for i, member := range detail.Members {
		item.Members[i] = groupops.Member{LUID: member.LUID, Name: member.Name, SiteRole: member.SiteRole}
	}
	return item, err
}

func (a adminGroupAdapter) CreateGroup(ctx context.Context, input groupops.CreateRequest) (groupops.Record, error) {
	item, err := a.Adapter.CreateGroup(ctx, tableauadmin.CreateGroupRequest{Name: input.Name, MinimumSiteRole: input.MinimumSiteRole, ExternalUserEnabled: input.ExternalUserEnabled})
	return adminGroupRecord(item), err
}

func (a adminGroupAdapter) UpdateGroup(ctx context.Context, luid string, input groupops.UpdateRequest) (groupops.Record, error) {
	item, err := a.Adapter.UpdateGroup(ctx, luid, tableauadmin.UpdateGroupRequest{Name: input.Name, MinimumSiteRole: input.MinimumSiteRole, ExternalUserEnabled: input.ExternalUserEnabled})
	return adminGroupRecord(item), err
}

func (a adminGroupAdapter) AddGroupUser(ctx context.Context, groupLUID, userLUID string) (string, error) {
	item, err := a.Adapter.AddGroupUser(ctx, groupLUID, userLUID)
	return item.RequestID, err
}

func (a adminGroupAdapter) RemoveGroupUser(ctx context.Context, groupLUID, userLUID string) (string, error) {
	item, err := a.Adapter.RemoveGroupUser(ctx, groupLUID, userLUID)
	return item.RequestID, err
}

func (a adminGroupAdapter) DeleteGroup(ctx context.Context, luid string) (groupops.DeleteResult, error) {
	item, err := a.Adapter.DeleteGroup(ctx, luid)
	return groupops.DeleteResult{Status: item.Status, GroupLUID: item.ResourceLUID, TableauRequestID: item.RequestID}, err
}
