package app

import (
	"context"
	"errors"

	groupcreate "github.com/ahillspace/tadx/actions/admin/group/create"
	groupdelete "github.com/ahillspace/tadx/actions/admin/group/delete"
	groupinspect "github.com/ahillspace/tadx/actions/admin/group/inspect"
	grouplist "github.com/ahillspace/tadx/actions/admin/group/list"
	groupmemberadd "github.com/ahillspace/tadx/actions/admin/group/member/add"
	groupmemberremove "github.com/ahillspace/tadx/actions/admin/group/member/remove"
	groupupdate "github.com/ahillspace/tadx/actions/admin/group/update"
	permissioninspect "github.com/ahillspace/tadx/actions/admin/permission/inspect"
	usercreate "github.com/ahillspace/tadx/actions/admin/user/create"
	userdelete "github.com/ahillspace/tadx/actions/admin/user/delete"
	userinspect "github.com/ahillspace/tadx/actions/admin/user/inspect"
	userlist "github.com/ahillspace/tadx/actions/admin/user/list"
	userupdate "github.com/ahillspace/tadx/actions/admin/user/update"
	"github.com/ahillspace/tadx/internal/catalog"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	resourceadmin "github.com/ahillspace/tadx/internal/resources/admin"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
	tableaucatalog "github.com/ahillspace/tadx/internal/tableau/catalog"
)

// remoteAdminCommands composes administration actions without owning CLI behavior.
type remoteAdminCommands struct{ runtime *runtimeDependencies }

// newRemoteAdminCommands creates the administration composition service used by root wiring.
func newRemoteAdminCommands(runtime *runtimeDependencies) *remoteAdminCommands {
	return &remoteAdminCommands{runtime: runtime}
}

// dependencies returns every action executor required by the administration CLI tree.
func (c *remoteAdminCommands) dependencies() *admincli.Dependencies {
	return &admincli.Dependencies{
		PermissionCapabilities: tableauadmin.PermissionCapabilities,
		UserLister:             c, UserInspector: c, UserCreator: c, UserUpdater: c, UserDeleter: c,
		GroupLister: c, GroupInspector: c, GroupCreator: c, GroupUpdater: c, GroupDeleter: c,
		GroupMemberAdder: c, GroupMemberRemover: c,
		PermissionInspector: c, PermissionCreator: c, PermissionDeleter: c,
	}
}

type adminConnection struct {
	environment config.Environment
	adapter     *resourceadmin.Adapter
	inventory   tableaucatalog.Executor
}

func (c *remoteAdminCommands) connect(ctx context.Context, alias string, explicit bool) (adminConnection, error) {
	connection, err := c.runtime.tableauConnection(ctx, alias, explicit)
	if err != nil {
		return adminConnection{environment: connection.environment}, err
	}
	client := tableauadmin.NewClient(connection.transport, connection.session, connection.environment.URL)
	return adminConnection{environment: connection.environment, adapter: resourceadmin.NewAdapter(client), inventory: catalogTableauExecutor{transport: connection.transport, session: connection.session, serverURL: connection.environment.URL, siteLUID: connection.session.SiteLUID()}}, nil
}

func (c *remoteAdminCommands) ListAdminUsers(ctx context.Context, input userlist.Input) (result userlist.Output, resultErr error) {
	if err := userlist.ValidateInput(input); err != nil {
		return userlist.Output{}, err
	}
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return userlist.Output{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
		if err := userlist.ValidateContinuation(input); err != nil {
			return userlist.Output{}, err
		}
	}
	defer func() {
		if resultErr == nil {
			resultErr = validateInventoryAll(input.All, result.Source)
		}
	}()
	if input.Catalog || legacyInventorySnapshot(input.Cursor) {
		environment, site, err := c.resolveCatalogTarget(input.Environment)
		if err != nil {
			return userlist.Output{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &catalogUserListReader{store: c.catalogStore(input.Environment), environment: environment, site: site}
		output, err := userlist.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	filter, err := tableauadmin.UserListFilter(tableauadmin.ListUsersRequest{Name: input.Name, SiteRole: input.SiteRole})
	if err != nil {
		return userlist.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return userlist.Output{}, remoteSetupError("admin.user.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	if input.All {
		observedAt := c.runtime.now().UTC()
		inventory, err := collectResourceInventory(ctx, connection.inventory, c.catalogStore(input.Environment), tableaucatalog.ScopeUsers, input.Environment, input.Site, observedAt, inventoryCollectionOptions{MaxConcurrency: connection.environment.CatalogMaxConcurrency, Filter: filter})
		if err != nil {
			return userlist.Output{}, inventoryRefreshError("user.list", input.Environment, input.Site, err)
		}
		reader := inventory.memoryReader()
		reader.allowContinuation = true
		output, err := userlist.New(reader).Execute(ctx, input)
		if err != nil {
			return output, err
		}
		if inventory.catalogErr != nil {
			output.Source = inventory.warningSource(observedAt)
			output.Help = append(output.Help, inventory.warningHelp())
		} else if inventory.filtered {
			output.Source = liveSource(c.runtime.now)
		} else {
			output.Source = liveInventorySource(observedAt, inventory.published.GenerationID)
		}
		output.RequestID = finalRequestID(inventory.requestIDs)
		return output, nil
	}
	output, err := userlist.New(adminUserListReader{connection.adapter}).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	return output, nil
}

func adminUserListIsUnfiltered(input userlist.Input) bool {
	return input.Name == "" && input.SiteRole == ""
}

func (c *remoteAdminCommands) InspectAdminUser(ctx context.Context, input userinspect.Input) (userinspect.Output, error) {
	if err := userinspect.ValidateInput(input); err != nil {
		return userinspect.Output{}, err
	}
	if input.Catalog {
		environment, site, err := c.resolveCatalogTarget(input.Environment)
		if err != nil {
			return userinspect.Output{}, err
		}
		input.Environment, input.Site = environment, site
		resolver := &catalogUserGetResolver{store: c.catalogStore(input.Environment), environment: environment, site: site}
		output, err := userinspect.New(resolver).Execute(ctx, input)
		if err == nil {
			output.Source = resolver.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return userinspect.Output{}, remoteSetupError("admin.user.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	output, err := userinspect.New(adminUserGetResolver{connection.adapter}).Execute(ctx, input)
	if err != nil {
		return output, adminActionError("admin.user.inspect", input.Environment, input.Site, err)
	}
	observedAt := c.runtime.now().UTC()
	output.Source = liveSource(c.runtime.now)
	entry, encodeErr := resourceEntry(input.Environment, input.Site, "user", output.User.LUID, output.User.Name, "", "", "detail", observedAt, output.User)
	if encodeErr == nil {
		writeThrough(c.catalogStore(input.Environment), []catalog.ResourceEntry{entry})
	}
	return output, nil
}

func (c *remoteAdminCommands) CreateAdminUser(ctx context.Context, input usercreate.Input, preview bool) (usercreate.Output, error) {
	if err := usercreate.ValidateInput(input); err != nil {
		return usercreate.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return usercreate.Output{}, remoteSetupError("admin.user.create", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := adminUserCreateAdapter{connection.adapter}
	output, err := usercreate.New(adapter, adapter).Execute(ctx, input, preview)
	return output, adminActionError("admin.user.create", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) UpdateAdminUser(ctx context.Context, input userupdate.Input, preview bool) (userupdate.Output, error) {
	if err := userupdate.ValidateInput(input); err != nil {
		return userupdate.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return userupdate.Output{}, remoteSetupError("admin.user.update", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	if input.Username != "" {
		user, resolveErr := connection.adapter.ResolveUser(ctx, resourceadmin.UserSelector{Username: input.Username})
		if resolveErr != nil {
			return userupdate.Output{}, adminActionError("admin.user.update", input.Environment, input.Site, resolveErr)
		}
		input.UserLUID, input.Username = user.LUID, ""
	}
	input.TargetResolved = true
	adapter := adminUserUpdateAdapter{connection.adapter}
	output, err := userupdate.New(adapter, adapter).Execute(ctx, input, preview)
	return output, adminActionError("admin.user.update", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) DeleteAdminUser(ctx context.Context, input userdelete.Input, preview bool) (userdelete.Output, error) {
	if err := userdelete.ValidateInput(input); err != nil {
		return userdelete.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return userdelete.Output{}, remoteSetupError("admin.user.delete", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	if input.Username != "" {
		user, resolveErr := connection.adapter.ResolveUser(ctx, resourceadmin.UserSelector{Username: input.Username})
		if resolveErr != nil {
			return userdelete.Output{}, adminActionError("admin.user.delete", input.Environment, input.Site, resolveErr)
		}
		input.UserLUID, input.Username = user.LUID, ""
	}
	input.TargetResolved = true
	adapter := adminUserDeleteAdapter{connection.adapter}
	output, err := userdelete.New(adapter, adapter).Execute(ctx, input, preview)
	return output, adminActionError("admin.user.delete", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) ListAdminGroups(ctx context.Context, input grouplist.Input) (result grouplist.Output, resultErr error) {
	if err := grouplist.ValidateInput(input); err != nil {
		return grouplist.Output{}, err
	}
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return grouplist.Output{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
		if err := grouplist.ValidateContinuation(input); err != nil {
			return grouplist.Output{}, err
		}
	}
	defer func() {
		if resultErr == nil {
			resultErr = validateInventoryAll(input.All, result.Source)
		}
	}()
	if input.Catalog || legacyInventorySnapshot(input.Cursor) {
		environment, site, err := c.resolveCatalogTarget(input.Environment)
		if err != nil {
			return grouplist.Output{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &catalogGroupListReader{store: c.catalogStore(input.Environment), environment: environment, site: site}
		output, err := grouplist.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	filter, err := tableauadmin.GroupListFilter(tableauadmin.ListGroupsRequest{Name: input.Name, Domain: input.Domain})
	if err != nil {
		return grouplist.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return grouplist.Output{}, remoteSetupError("admin.group.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	if input.All {
		observedAt := c.runtime.now().UTC()
		inventory, err := collectResourceInventory(ctx, connection.inventory, c.catalogStore(input.Environment), tableaucatalog.ScopeGroups, input.Environment, input.Site, observedAt, inventoryCollectionOptions{MaxConcurrency: connection.environment.CatalogMaxConcurrency, Filter: filter})
		if err != nil {
			return grouplist.Output{}, inventoryRefreshError("group.list", input.Environment, input.Site, err)
		}
		reader := inventory.memoryReader()
		reader.allowContinuation = true
		output, err := grouplist.New(reader).Execute(ctx, input)
		if err != nil {
			return output, err
		}
		if inventory.catalogErr != nil {
			output.Source = inventory.warningSource(observedAt)
			output.Help = append(output.Help, inventory.warningHelp())
		} else if inventory.filtered {
			output.Source = liveSource(c.runtime.now)
		} else {
			output.Source = liveInventorySource(observedAt, inventory.published.GenerationID)
		}
		output.RequestID = finalRequestID(inventory.requestIDs)
		return output, nil
	}
	output, err := grouplist.New(adminGroupListReader{connection.adapter}).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	return output, nil
}

func adminGroupListIsUnfiltered(input grouplist.Input) bool {
	return input.Name == "" && input.Domain == ""
}

func (c *remoteAdminCommands) InspectAdminGroup(ctx context.Context, input groupinspect.Input) (groupinspect.Output, error) {
	if err := groupinspect.ValidateInput(input); err != nil {
		return groupinspect.Output{}, err
	}
	if input.Catalog {
		environment, site, err := c.resolveCatalogTarget(input.Environment)
		if err != nil {
			return groupinspect.Output{}, err
		}
		input.Environment, input.Site = environment, site
		resolver := &catalogGroupGetResolver{store: c.catalogStore(input.Environment), environment: environment, site: site}
		output, err := groupinspect.New(resolver).Execute(ctx, input)
		if err == nil {
			output.Source = resolver.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return groupinspect.Output{}, remoteSetupError("admin.group.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	output, err := groupinspect.New(adminGroupGetResolver{connection.adapter}).Execute(ctx, input)
	if err != nil {
		return output, adminActionError("admin.group.inspect", input.Environment, input.Site, err)
	}
	observedAt := c.runtime.now().UTC()
	output.Source = liveSource(c.runtime.now)
	entry, encodeErr := resourceEntry(input.Environment, input.Site, "group", output.Group.LUID, output.Group.Name, "", "", "detail", observedAt, output.Group)
	if encodeErr == nil {
		writeThrough(c.catalogStore(input.Environment), []catalog.ResourceEntry{entry})
	}
	return output, nil
}

func (c *remoteAdminCommands) CreateAdminGroup(ctx context.Context, input groupcreate.Input, preview bool) (groupcreate.Output, error) {
	if err := groupcreate.ValidateInput(input); err != nil {
		return groupcreate.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return groupcreate.Output{}, remoteSetupError("admin.group.create", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := adminGroupCreateAdapter{connection.adapter}
	output, err := groupcreate.New(adapter, adapter).Execute(ctx, input, preview)
	return output, adminActionError("admin.group.create", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) UpdateAdminGroup(ctx context.Context, input groupupdate.Input, preview bool) (groupupdate.Output, error) {
	if err := groupupdate.ValidateInput(input); err != nil {
		return groupupdate.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return groupupdate.Output{}, remoteSetupError("admin.group.update", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := adminGroupUpdateAdapter{connection.adapter}
	output, err := groupupdate.New(adapter, adapter, adapter).Execute(ctx, input, preview)
	return output, adminActionError("admin.group.update", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) DeleteAdminGroup(ctx context.Context, input groupdelete.Input, preview bool) (groupdelete.Output, error) {
	if err := groupdelete.ValidateInput(input); err != nil {
		return groupdelete.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return groupdelete.Output{}, remoteSetupError("admin.group.delete", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := adminGroupDeleteAdapter{connection.adapter}
	output, err := groupdelete.New(adapter, adapter).Execute(ctx, input, preview)
	return output, adminActionError("admin.group.delete", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) AddAdminGroupMember(ctx context.Context, input groupmemberadd.Input, preview bool) (groupmemberadd.Output, error) {
	if err := groupmemberadd.ValidateInput(input); err != nil {
		return groupmemberadd.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return groupmemberadd.Output{}, remoteSetupError("admin.group.member.add", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := adminGroupMemberAddAdapter{adapter: connection.adapter}
	out, err := groupmemberadd.New(adapter, adapter).Execute(ctx, input, preview)
	return out, adminActionError("admin.group.member.add", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) RemoveAdminGroupMember(ctx context.Context, input groupmemberremove.Input, preview bool) (groupmemberremove.Output, error) {
	if err := groupmemberremove.ValidateInput(input); err != nil {
		return groupmemberremove.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return groupmemberremove.Output{}, remoteSetupError("admin.group.member.remove", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := adminGroupMemberRemoveAdapter{adapter: connection.adapter}
	out, err := groupmemberremove.New(adapter, adapter).Execute(ctx, input, preview)
	return out, adminActionError("admin.group.member.remove", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) InspectAdminPermission(ctx context.Context, input permissioninspect.Input) (permissioninspect.Output, error) {
	if err := permissioninspect.ValidateInput(input); err != nil {
		return permissioninspect.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return permissioninspect.Output{}, remoteSetupError("admin.permission.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	if input.PrincipalUsername != "" {
		user, resolveErr := connection.adapter.ResolveUser(ctx, resourceadmin.UserSelector{Username: input.PrincipalUsername})
		if resolveErr != nil {
			return permissioninspect.Output{}, adminActionError("admin.permission.inspect", input.Environment, input.Site, resolveErr)
		}
		input.PrincipalLUID, input.PrincipalUsername = user.LUID, ""
	}
	output, err := permissioninspect.New(adminPermissionReader{connection.adapter}).Execute(ctx, input)
	return output, adminActionError("admin.permission.inspect", input.Environment, input.Site, err)
}

func adminActionError(operation, environment, site string, err error) error {
	if err == nil {
		return nil
	}
	var structured *errs.Error
	if errors.As(err, &structured) {
		return err
	}
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact administration target and upstream response, then retry.")
	return &errs.Error{ID: operation + ".failed", Kind: errs.KindOperation, Operation: operation, Environment: environment, Site: site, Summary: "Tableau administration operation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
}

type adminUserListReader struct{ adapter *resourceadmin.Adapter }

func (a adminUserListReader) ListUsers(ctx context.Context, input userlist.PageRequest) (userlist.Page, error) {
	page, err := a.adapter.ListUsers(ctx, tableauadmin.ListUsersRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, SiteRole: input.SiteRole})
	items := make([]userlist.User, len(page.Items))
	for i, item := range page.Items {
		items[i] = toUserList(item)
	}
	return userlist.Page{Number: page.Number, Size: page.Size, Total: page.Total, Users: items, RequestID: page.RequestID}, err
}
func toUserList(item tableauadmin.User) userlist.User {
	return userlist.User{LUID: item.LUID, Name: item.Name, FullName: item.FullName, Email: item.Email, SiteRole: item.SiteRole, LastLogin: item.LastLogin, AuthSetting: item.AuthSetting, Domain: item.Domain}
}

type adminUserGetResolver struct{ adapter *resourceadmin.Adapter }

func (a adminUserGetResolver) ResolveUser(ctx context.Context, selector userinspect.Selector) (userinspect.User, error) {
	item, err := a.adapter.ResolveUser(ctx, resourceadmin.UserSelector{LUID: selector.LUID, Username: selector.Username})
	return toUserGet(item), err
}
func toUserGet(item tableauadmin.User) userinspect.User {
	return userinspect.User{LUID: item.LUID, Name: item.Name, FullName: item.FullName, Email: item.Email, SiteRole: item.SiteRole, LastLogin: item.LastLogin, ExternalAuthUserID: item.ExternalAuthUserID, AuthSetting: item.AuthSetting, IdentityPoolName: item.IdentityPoolName, IdPConfigurationID: item.IdPConfigurationID, Language: item.Language, Locale: item.Locale, Domain: item.Domain, RequestID: item.RequestID}
}

type adminUserCreateAdapter struct{ adapter *resourceadmin.Adapter }

func (a adminUserCreateAdapter) FindUsers(ctx context.Context, name string) ([]usercreate.User, error) {
	items, err := a.adapter.FindUsers(ctx, name)
	out := make([]usercreate.User, len(items))
	for i, v := range items {
		out[i] = usercreate.User{LUID: v.LUID, Name: v.Name, SiteRole: v.SiteRole, AuthSetting: v.AuthSetting, IdPConfigurationID: v.IdPConfigurationID}
	}
	return out, err
}
func (a adminUserCreateAdapter) CreateUser(ctx context.Context, input usercreate.Request) (usercreate.User, error) {
	item, err := a.adapter.CreateUser(ctx, tableauadmin.CreateUserRequest{Name: input.Name, SiteRole: input.SiteRole, AuthSetting: input.AuthSetting, IdentityPoolName: input.IdentityPoolName, IdPConfigurationID: input.IdPConfigurationID, Email: input.Email, Language: input.Language, Locale: input.Locale})
	return usercreate.User{LUID: item.LUID, Name: item.Name, SiteRole: item.SiteRole, AuthSetting: item.AuthSetting, IdPConfigurationID: item.IdPConfigurationID, RequestID: item.RequestID, MutationStatus: item.MutationStatus}, err
}

type adminUserUpdateAdapter struct{ adapter *resourceadmin.Adapter }

func (a adminUserUpdateAdapter) ResolveUser(ctx context.Context, luid string) (userupdate.User, error) {
	item, err := a.adapter.ResolveUser(ctx, resourceadmin.UserSelector{LUID: luid})
	return userupdate.User{LUID: item.LUID, Name: item.Name, FullName: item.FullName, Email: item.Email, SiteRole: item.SiteRole, AuthSetting: item.AuthSetting, IdentityPoolName: item.IdentityPoolName, IdPConfigurationID: item.IdPConfigurationID, Language: item.Language, Locale: item.Locale}, err
}
func (a adminUserUpdateAdapter) UpdateUser(ctx context.Context, luid string, input userupdate.Request) (userupdate.User, error) {
	item, err := a.adapter.UpdateUser(ctx, luid, tableauadmin.UpdateUserRequest{FullName: input.FullName, Email: input.Email, SiteRole: input.SiteRole, AuthSetting: input.AuthSetting, IdentityPoolName: input.IdentityPoolName, IdPConfigurationID: input.IdPConfigurationID, Language: input.Language, Locale: input.Locale})
	return userupdate.User{LUID: item.LUID, Name: item.Name, FullName: item.FullName, Email: item.Email, SiteRole: item.SiteRole, AuthSetting: item.AuthSetting, IdentityPoolName: item.IdentityPoolName, IdPConfigurationID: item.IdPConfigurationID, Language: item.Language, Locale: item.Locale, RequestID: item.RequestID, MutationStatus: item.MutationStatus}, err
}

type adminUserDeleteAdapter struct{ adapter *resourceadmin.Adapter }

func (a adminUserDeleteAdapter) ResolveUser(ctx context.Context, luid string) (userdelete.User, error) {
	item, err := a.adapter.ResolveUser(ctx, resourceadmin.UserSelector{LUID: luid})
	return userdelete.User{LUID: item.LUID, Name: item.Name, SiteRole: item.SiteRole}, err
}
func (a adminUserDeleteAdapter) DeleteUser(ctx context.Context, luid string) (userdelete.Result, error) {
	item, err := a.adapter.DeleteUser(ctx, luid)
	return userdelete.Result{Status: item.Status, UserLUID: item.ResourceLUID, TableauRequestID: item.RequestID}, err
}

type adminGroupListReader struct{ adapter *resourceadmin.Adapter }

func (a adminGroupListReader) ListGroups(ctx context.Context, input grouplist.PageRequest) (grouplist.Page, error) {
	page, err := a.adapter.ListGroups(ctx, tableauadmin.ListGroupsRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, Domain: input.Domain})
	items := make([]grouplist.Group, len(page.Items))
	for i, v := range page.Items {
		items[i] = grouplist.Group{LUID: v.LUID, Name: v.Name, Domain: v.Domain, MinimumSiteRole: v.MinimumSiteRole, GrantLicenseMode: v.GrantLicenseMode, ExternalUserEnabled: v.ExternalUserEnabled}
	}
	return grouplist.Page{Number: page.Number, Size: page.Size, Total: page.Total, Groups: items, RequestID: page.RequestID}, err
}

type adminGroupGetResolver struct{ adapter *resourceadmin.Adapter }

func (a adminGroupGetResolver) ResolveGroup(ctx context.Context, selector groupinspect.Selector, members bool) (groupinspect.Group, error) {
	detail, err := a.adapter.ResolveGroup(ctx, resourceadmin.GroupSelector{LUID: selector.LUID, Name: selector.Name}, members)
	items := make([]groupinspect.Member, len(detail.Members))
	for i, v := range detail.Members {
		items[i] = groupinspect.Member{LUID: v.LUID, Name: v.Name, SiteRole: v.SiteRole}
	}
	g := detail.Group
	return groupinspect.Group{LUID: g.LUID, Name: g.Name, Domain: g.Domain, MinimumSiteRole: g.MinimumSiteRole, GrantLicenseMode: g.GrantLicenseMode, ExternalUserEnabled: g.ExternalUserEnabled, Members: items, RequestID: g.RequestID}, err
}

type adminGroupCreateAdapter struct{ adapter *resourceadmin.Adapter }

func (a adminGroupCreateAdapter) FindGroups(ctx context.Context, name string) ([]groupcreate.Group, error) {
	items, err := a.adapter.FindGroups(ctx, name)
	out := make([]groupcreate.Group, len(items))
	for i, v := range items {
		out[i] = groupcreate.Group{LUID: v.LUID, Name: v.Name}
	}
	return out, err
}
func (a adminGroupCreateAdapter) CreateGroup(ctx context.Context, input groupcreate.Request) (groupcreate.Group, error) {
	item, err := a.adapter.CreateGroup(ctx, tableauadmin.CreateGroupRequest{Name: input.Name, MinimumSiteRole: input.MinimumSiteRole, ExternalUserEnabled: input.ExternalUserEnabled})
	return groupcreate.Group{LUID: item.LUID, Name: item.Name, RequestID: item.RequestID, MutationStatus: item.MutationStatus}, err
}

type adminGroupUpdateAdapter struct{ adapter *resourceadmin.Adapter }

func (a adminGroupUpdateAdapter) ResolveGroup(ctx context.Context, luid string, members bool) (groupupdate.Group, error) {
	detail, err := a.adapter.ResolveGroup(ctx, resourceadmin.GroupSelector{LUID: luid}, members)
	items := make([]groupupdate.Member, len(detail.Members))
	for i, v := range detail.Members {
		items[i] = groupupdate.Member{LUID: v.LUID, Name: v.Name}
	}
	g := detail.Group
	return groupupdate.Group{LUID: g.LUID, Name: g.Name, Domain: g.Domain, MinimumSiteRole: g.MinimumSiteRole, ExternalUserEnabled: g.ExternalUserEnabled, Members: items}, err
}
func (a adminGroupUpdateAdapter) UpdateGroup(ctx context.Context, luid string, input groupupdate.Request) (groupupdate.Group, error) {
	item, err := a.adapter.UpdateGroup(ctx, luid, tableauadmin.UpdateGroupRequest{Name: input.Name, MinimumSiteRole: input.MinimumSiteRole, ExternalUserEnabled: input.ExternalUserEnabled})
	return groupupdate.Group{LUID: item.LUID, Name: item.Name, Domain: item.Domain, MinimumSiteRole: item.MinimumSiteRole, ExternalUserEnabled: item.ExternalUserEnabled, RequestID: item.RequestID, MutationStatus: item.MutationStatus}, err
}
func (a adminGroupUpdateAdapter) AddGroupUser(ctx context.Context, group, user string) (string, error) {
	item, err := a.adapter.AddGroupUser(ctx, group, user)
	return item.RequestID, err
}
func (a adminGroupUpdateAdapter) RemoveGroupUser(ctx context.Context, group, user string) (string, error) {
	item, err := a.adapter.RemoveGroupUser(ctx, group, user)
	return item.RequestID, err
}

type adminGroupDeleteAdapter struct{ adapter *resourceadmin.Adapter }

type adminGroupMemberAddAdapter struct{ adapter *resourceadmin.Adapter }

func (a adminGroupMemberAddAdapter) ResolveUsername(ctx context.Context, username string) (groupmemberadd.Member, error) {
	user, err := a.adapter.ResolveUser(ctx, resourceadmin.UserSelector{Username: username})
	return groupmemberadd.Member{LUID: user.LUID, Name: user.Name}, err
}

func (a adminGroupMemberAddAdapter) ResolveGroup(ctx context.Context, luid string) (groupmemberadd.Group, error) {
	detail, err := a.adapter.ResolveGroup(ctx, resourceadmin.GroupSelector{LUID: luid}, true)
	members := make([]groupmemberadd.Member, len(detail.Members))
	for i, v := range detail.Members {
		members[i] = groupmemberadd.Member{LUID: v.LUID, Name: v.Name}
	}
	return groupmemberadd.Group{LUID: detail.Group.LUID, Name: detail.Group.Name, Members: members}, err
}
func (a adminGroupMemberAddAdapter) AddGroupUser(ctx context.Context, group, user string) (groupmemberadd.Result, error) {
	item, err := a.adapter.AddGroupUser(ctx, group, user)
	return groupmemberadd.Result{Status: item.Status, GroupLUID: group, UserLUID: item.ResourceLUID, TableauRequestID: item.RequestID}, err
}

type adminGroupMemberRemoveAdapter struct{ adapter *resourceadmin.Adapter }

func (a adminGroupMemberRemoveAdapter) ResolveUsername(ctx context.Context, username string) (groupmemberremove.Member, error) {
	user, err := a.adapter.ResolveUser(ctx, resourceadmin.UserSelector{Username: username})
	return groupmemberremove.Member{LUID: user.LUID, Name: user.Name}, err
}

func (a adminGroupMemberRemoveAdapter) ResolveGroup(ctx context.Context, luid string) (groupmemberremove.Group, error) {
	detail, err := a.adapter.ResolveGroup(ctx, resourceadmin.GroupSelector{LUID: luid}, true)
	members := make([]groupmemberremove.Member, len(detail.Members))
	for i, v := range detail.Members {
		members[i] = groupmemberremove.Member{LUID: v.LUID, Name: v.Name}
	}
	return groupmemberremove.Group{LUID: detail.Group.LUID, Name: detail.Group.Name, Members: members}, err
}
func (a adminGroupMemberRemoveAdapter) RemoveGroupUser(ctx context.Context, group, user string) (groupmemberremove.Result, error) {
	item, err := a.adapter.RemoveGroupUser(ctx, group, user)
	return groupmemberremove.Result{Status: item.Status, GroupLUID: group, UserLUID: item.ResourceLUID, TableauRequestID: item.RequestID}, err
}

func (a adminGroupDeleteAdapter) ResolveGroup(ctx context.Context, luid string) (groupdelete.Group, error) {
	detail, err := a.adapter.ResolveGroup(ctx, resourceadmin.GroupSelector{LUID: luid}, false)
	g := detail.Group
	return groupdelete.Group{LUID: g.LUID, Name: g.Name, Domain: g.Domain}, err
}
func (a adminGroupDeleteAdapter) DeleteGroup(ctx context.Context, luid string) (groupdelete.Result, error) {
	item, err := a.adapter.DeleteGroup(ctx, luid)
	return groupdelete.Result{Status: item.Status, GroupLUID: item.ResourceLUID, TableauRequestID: item.RequestID}, err
}

type adminPermissionReader struct{ adapter *resourceadmin.Adapter }

func (a adminPermissionReader) GetPermissions(ctx context.Context, input permissioninspect.Input) (permissioninspect.PermissionSet, error) {
	item, err := a.adapter.GetPermissions(ctx, tableauadmin.PermissionRequest{ResourceKind: input.ResourceKind, ResourceLUID: input.ResourceLUID, DefaultFor: input.DefaultFor})
	rules := make([]permissioninspect.Rule, len(item.Rules))
	for i, v := range item.Rules {
		rules[i] = permissioninspect.Rule{PrincipalType: v.PrincipalType, PrincipalLUID: v.PrincipalLUID, Capability: v.Capability, Mode: v.Mode}
	}
	return permissioninspect.PermissionSet{ResourceKind: item.ResourceKind, ResourceLUID: item.ResourceLUID, Source: item.Source, ParentProjectLUID: item.ParentProjectLUID, Rules: rules, RequestID: item.RequestID}, err
}

var _ admincli.UserLister = (*remoteAdminCommands)(nil)
var _ admincli.UserInspector = (*remoteAdminCommands)(nil)
var _ admincli.UserCreator = (*remoteAdminCommands)(nil)
var _ admincli.UserUpdater = (*remoteAdminCommands)(nil)
var _ admincli.UserDeleter = (*remoteAdminCommands)(nil)
var _ admincli.GroupLister = (*remoteAdminCommands)(nil)
var _ admincli.GroupInspector = (*remoteAdminCommands)(nil)
var _ admincli.GroupCreator = (*remoteAdminCommands)(nil)
var _ admincli.GroupUpdater = (*remoteAdminCommands)(nil)
var _ admincli.GroupDeleter = (*remoteAdminCommands)(nil)
var _ admincli.GroupMemberAdder = (*remoteAdminCommands)(nil)
var _ admincli.GroupMemberRemover = (*remoteAdminCommands)(nil)
var _ admincli.PermissionInspector = (*remoteAdminCommands)(nil)
