package admin

import (
	"context"

	user "github.com/ahillspace/tadx/actions/admin/user"
	tableau "github.com/ahillspace/tadx/internal/tableau/admin"
)

// UserPorts translates native user records and resolves fresh caller context.
type UserPorts struct {
	Adapter    *Adapter
	CallerLUID string
	ServerURL  string
}

func (p UserPorts) ListUsers(ctx context.Context, in user.ListPageRequest) (user.ListPage, error) {
	page, err := p.Adapter.ListUsers(ctx, tableau.ListUsersRequest{PageNumber: in.PageNumber, PageSize: in.PageSize, Name: in.Name, SiteRole: in.SiteRole})
	items := make([]user.Record, len(page.Items))
	for i, item := range page.Items {
		items[i] = userRecord(item)
	}
	return user.ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Users: items, RequestID: page.RequestID}, err
}

func (p UserPorts) ResolveUser(ctx context.Context, selector user.Selector) (user.Record, error) {
	item, err := p.Adapter.ResolveUser(ctx, UserSelector{LUID: selector.LUID, Username: selector.Username})
	return userRecord(item), err
}

func userRecord(item tableau.User) user.Record {
	return user.Record{LUID: item.LUID, Name: item.Name, FullName: item.FullName, Email: item.Email, SiteRole: item.SiteRole, LastLogin: item.LastLogin, ExternalAuthUserID: item.ExternalAuthUserID, AuthSetting: item.AuthSetting, IdentityPoolName: item.IdentityPoolName, IdPConfigurationID: item.IdPConfigurationID, Language: item.Language, Locale: item.Locale, Domain: item.Domain, RequestID: item.RequestID, MutationStatus: item.MutationStatus, PresentFields: item.PresentFields}
}

func (p UserPorts) UserExists(ctx context.Context, name string) (bool, error) {
	return p.Adapter.UserExists(ctx, name)
}

func (p UserPorts) CreateUser(ctx context.Context, in user.CreateRequest) (user.Record, error) {
	item, err := p.Adapter.CreateUser(ctx, tableau.CreateUserRequest{Name: in.Name, SiteRole: in.SiteRole, AuthSetting: in.AuthSetting, IdentityPoolName: in.IdentityPoolName, IdPConfigurationID: in.IdPConfigurationID, Email: in.Email, Language: in.Language, Locale: in.Locale})
	return userRecord(item), err
}

func (p UserPorts) UpdateUser(ctx context.Context, luid string, in user.UpdateRequest) (user.Record, error) {
	item, err := p.Adapter.UpdateUser(ctx, luid, tableau.UpdateUserRequest{FullName: in.FullName, Email: in.Email, SiteRole: in.SiteRole, AuthSetting: in.AuthSetting, IdentityPoolName: in.IdentityPoolName, IdPConfigurationID: in.IdPConfigurationID, Language: in.Language, Locale: in.Locale})
	return userRecord(item), err
}

func (p UserPorts) DeleteUser(ctx context.Context, luid string) (user.DeleteResult, error) {
	item, err := p.Adapter.DeleteUser(ctx, luid)
	return user.DeleteResult{Status: item.Status, UserLUID: item.ResourceLUID, TableauRequestID: item.RequestID}, err
}

func (p UserPorts) CallerSiteRole(ctx context.Context) (string, error) {
	if p.CallerLUID == "" {
		return "", nil
	}
	caller, err := p.Adapter.ResolveUser(ctx, UserSelector{LUID: p.CallerLUID})
	return caller.SiteRole, err
}

func (p UserPorts) ResolveUsername(ctx context.Context, username string) (string, error) {
	item, err := p.Adapter.ResolveUser(ctx, UserSelector{Username: username})
	return item.LUID, err
}

func (p UserPorts) ValidateUpdate(ctx context.Context, in user.UpdateRequest) error {
	return user.ValidateUpdatePolicy(ctx, p.ServerURL, p.CallerSiteRole, in)
}
