package app

import (
	"context"
	"errors"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	groupmember "github.com/ahillspace/tadx/actions/admin/group/member"
	permissioninspect "github.com/ahillspace/tadx/actions/admin/permission/inspect"
	userops "github.com/ahillspace/tadx/actions/admin/user"
	"github.com/ahillspace/tadx/internal/cache"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	resourceadmin "github.com/ahillspace/tadx/internal/resources/admin"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
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
	inventory   tableaucache.Executor
}

func (c *remoteAdminCommands) connect(ctx context.Context, alias string, explicit bool) (adminConnection, error) {
	connection, err := c.runtime.tableauConnection(ctx, alias, explicit)
	if err != nil {
		return adminConnection{environment: connection.environment}, err
	}
	client := tableauadmin.NewClient(connection.transport, connection.session, connection.environment.URL)
	return adminConnection{environment: connection.environment, adapter: resourceadmin.NewAdapter(client, c.runtime.checkManagedCapability), inventory: cacheTableauExecutor{checkCapability: c.runtime.checkManagedCapability, transport: connection.transport, session: connection.session, serverURL: connection.environment.URL, siteLUID: connection.session.SiteLUID()}}, nil
}

func (c *remoteAdminCommands) ListAdminUsers(ctx context.Context, input userops.ListInput) (result userops.ListOutput, resultErr error) {
	if err := userops.ValidateListInput(&input); err != nil {
		return userops.ListOutput{}, err
	}
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return userops.ListOutput{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
		if err := userops.ValidateListContinuation(&input); err != nil {
			return userops.ListOutput{}, err
		}
	}
	defer func() {
		if resultErr == nil {
			resultErr = validateInventoryAll(input.All, result.Source)
		}
	}()
	if input.Cache || legacyInventorySnapshot(input.Cursor) {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return userops.ListOutput{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &cacheUserListReader{checkCapability: c.runtime.checkManagedCapability, store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := userops.List(ctx, reader, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	filter, err := tableauadmin.UserListFilter(tableauadmin.ListUsersRequest{Name: input.Name, SiteRole: input.SiteRole})
	if err != nil {
		return userops.ListOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return userops.ListOutput{}, remoteSetupError("admin.user.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	if input.All {
		observedAt := c.runtime.now().UTC()
		inventory, err := collectResourceInventory(ctx, connection.inventory, c.cacheStore(input.Environment), tableaucache.ScopeUsers, input.Environment, input.Site, observedAt, inventoryCollectionOptions{MaxConcurrency: connection.environment.CacheMaxConcurrency, Filter: filter})
		if err != nil {
			return userops.ListOutput{}, inventoryRefreshError("user.list", input.Environment, input.Site, err)
		}
		reader := inventory.memoryReader()
		reader.allowContinuation = true
		output, err := userops.List(ctx, reader, input)
		if err != nil {
			return output, err
		}
		if inventory.cacheErr != nil {
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
	output, err := userops.List(ctx, adminUserAdapter{connection.adapter}, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	return output, nil
}

func (c *remoteAdminCommands) InspectAdminUser(ctx context.Context, input userops.InspectInput) (userops.InspectOutput, error) {
	if err := userops.ValidateInspectInput(input); err != nil {
		return userops.InspectOutput{}, err
	}
	if input.Cache {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return userops.InspectOutput{}, err
		}
		input.Environment, input.Site = environment, site
		resolver := &cacheUserGetResolver{checkCapability: c.runtime.checkManagedCapability, store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := userops.Inspect(ctx, resolver, input)
		if err == nil {
			output.Source = resolver.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return userops.InspectOutput{}, remoteSetupError("admin.user.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	output, err := userops.Inspect(ctx, adminUserAdapter{connection.adapter}, input)
	if err != nil {
		return output, adminActionError("admin.user.inspect", input.Environment, input.Site, err)
	}
	observedAt := c.runtime.now().UTC()
	output.Source = liveSource(c.runtime.now)
	entry, encodeErr := resourceEntry(input.Environment, input.Site, "user", output.User.LUID, output.User.Name, "", "", "detail", observedAt, output.User)
	if encodeErr == nil {
		writeThrough(c.cacheStore(input.Environment), []cache.ResourceEntry{entry})
	}
	return output, nil
}

func (c *remoteAdminCommands) CreateAdminUser(ctx context.Context, input userops.CreateInput, preview bool) (userops.CreateOutput, error) {
	if err := userops.ValidateCreateInput(input); err != nil {
		return userops.CreateOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return userops.CreateOutput{}, remoteSetupError("admin.user.create", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := adminUserAdapter{connection.adapter}
	output, err := userops.Create(ctx, adapter, adapter, input, preview)
	return output, adminActionError("admin.user.create", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) UpdateAdminUser(ctx context.Context, input userops.UpdateInput, preview bool) (userops.UpdateOutput, error) {
	if err := userops.ValidateUpdateInput(input); err != nil {
		return userops.UpdateOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return userops.UpdateOutput{}, remoteSetupError("admin.user.update", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	if input.Username != "" {
		user, resolveErr := connection.adapter.ResolveUser(ctx, resourceadmin.UserSelector{Username: input.Username})
		if resolveErr != nil {
			return userops.UpdateOutput{}, adminActionError("admin.user.update", input.Environment, input.Site, resolveErr)
		}
		input.UserLUID, input.Username = user.LUID, ""
	}
	adapter := adminUserAdapter{connection.adapter}
	output, err := userops.Update(ctx, adapter, adapter, input, preview)
	return output, adminActionError("admin.user.update", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) DeleteAdminUser(ctx context.Context, input userops.DeleteInput, preview bool) (userops.DeleteOutput, error) {
	if err := userops.ValidateDeleteInput(input); err != nil {
		return userops.DeleteOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return userops.DeleteOutput{}, remoteSetupError("admin.user.delete", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	if input.Username != "" {
		user, resolveErr := connection.adapter.ResolveUser(ctx, resourceadmin.UserSelector{Username: input.Username})
		if resolveErr != nil {
			return userops.DeleteOutput{}, adminActionError("admin.user.delete", input.Environment, input.Site, resolveErr)
		}
		input.UserLUID, input.Username = user.LUID, ""
	}
	adapter := adminUserAdapter{connection.adapter}
	output, err := userops.Delete(ctx, adapter, adapter, input, preview)
	return output, adminActionError("admin.user.delete", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) ListAdminGroups(ctx context.Context, input groupops.ListInput) (result groupops.ListOutput, resultErr error) {
	if err := groupops.ValidateListInput(&input); err != nil {
		return groupops.ListOutput{}, err
	}
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return groupops.ListOutput{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
		if err := groupops.ValidateListContinuation(&input); err != nil {
			return groupops.ListOutput{}, err
		}
	}
	defer func() {
		if resultErr == nil {
			resultErr = validateInventoryAll(input.All, result.Source)
		}
	}()
	if input.Cache || legacyInventorySnapshot(input.Cursor) {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return groupops.ListOutput{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &cacheGroupListReader{checkCapability: c.runtime.checkManagedCapability, store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := groupops.List(ctx, reader, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	filter, err := tableauadmin.GroupListFilter(tableauadmin.ListGroupsRequest{Name: input.Name, Domain: input.Domain})
	if err != nil {
		return groupops.ListOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return groupops.ListOutput{}, remoteSetupError("admin.group.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	if input.All {
		observedAt := c.runtime.now().UTC()
		inventory, err := collectResourceInventory(ctx, connection.inventory, c.cacheStore(input.Environment), tableaucache.ScopeGroups, input.Environment, input.Site, observedAt, inventoryCollectionOptions{MaxConcurrency: connection.environment.CacheMaxConcurrency, Filter: filter})
		if err != nil {
			return groupops.ListOutput{}, inventoryRefreshError("group.list", input.Environment, input.Site, err)
		}
		reader := inventory.memoryReader()
		reader.allowContinuation = true
		output, err := groupops.List(ctx, reader, input)
		if err != nil {
			return output, err
		}
		if inventory.cacheErr != nil {
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
	output, err := groupops.List(ctx, adminGroupAdapter{connection.adapter}, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	return output, nil
}

func (c *remoteAdminCommands) InspectAdminGroup(ctx context.Context, input groupops.InspectInput) (groupops.InspectOutput, error) {
	if err := groupops.ValidateInspectInput(input); err != nil {
		return groupops.InspectOutput{}, err
	}
	if input.Cache {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return groupops.InspectOutput{}, err
		}
		input.Environment, input.Site = environment, site
		resolver := &cacheGroupGetResolver{checkCapability: c.runtime.checkManagedCapability, store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := groupops.Inspect(ctx, resolver, input)
		if err == nil {
			output.Source = resolver.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return groupops.InspectOutput{}, remoteSetupError("admin.group.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	output, err := groupops.Inspect(ctx, adminGroupAdapter{connection.adapter}, input)
	if err != nil {
		return output, adminActionError("admin.group.inspect", input.Environment, input.Site, err)
	}
	observedAt := c.runtime.now().UTC()
	output.Source = liveSource(c.runtime.now)
	entry, encodeErr := resourceEntry(input.Environment, input.Site, "group", output.Group.LUID, output.Group.Name, "", "", "detail", observedAt, struct {
		groupops.Record
		MembersFetched bool `json:"members_fetched"`
	}{Record: output.Group, MembersFetched: input.IncludeMembers})
	if encodeErr == nil {
		writeThrough(c.cacheStore(input.Environment), []cache.ResourceEntry{entry})
	}
	return output, nil
}

func (c *remoteAdminCommands) CreateAdminGroup(ctx context.Context, input groupops.CreateInput, preview bool) (groupops.CreateOutput, error) {
	if err := groupops.ValidateCreateInput(input); err != nil {
		return groupops.CreateOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return groupops.CreateOutput{}, remoteSetupError("admin.group.create", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := adminGroupAdapter{connection.adapter}
	output, err := groupops.Create(ctx, adapter, adapter, input, preview)
	return output, adminActionError("admin.group.create", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) UpdateAdminGroup(ctx context.Context, input groupops.UpdateInput, preview bool) (groupops.UpdateOutput, error) {
	if err := groupops.ValidateUpdateInput(&input); err != nil {
		return groupops.UpdateOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return groupops.UpdateOutput{}, remoteSetupError("admin.group.update", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := adminGroupAdapter{connection.adapter}
	output, err := groupops.Update(ctx, adapter, adapter, adapter, input, preview)
	return output, adminActionError("admin.group.update", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) DeleteAdminGroup(ctx context.Context, input groupops.DeleteInput, preview bool) (groupops.DeleteOutput, error) {
	if err := groupops.ValidateDeleteInput(input); err != nil {
		return groupops.DeleteOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return groupops.DeleteOutput{}, remoteSetupError("admin.group.delete", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := adminGroupAdapter{connection.adapter}
	output, err := groupops.Delete(ctx, adapter, adapter, input, preview)
	return output, adminActionError("admin.group.delete", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) AddAdminGroupMember(ctx context.Context, input groupmember.Input, preview bool) (groupmember.Output, error) {
	if err := groupmember.ValidateAddInput(input); err != nil {
		return groupmember.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return groupmember.Output{}, remoteSetupError("admin.group.member.add", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := adminGroupMemberAdapter{adapter: connection.adapter}
	out, err := groupmember.Add(ctx, adapter, adapter, input, preview)
	return out, adminActionError("admin.group.member.add", input.Environment, input.Site, err)
}

func (c *remoteAdminCommands) RemoveAdminGroupMember(ctx context.Context, input groupmember.Input, preview bool) (groupmember.Output, error) {
	if err := groupmember.ValidateRemoveInput(input); err != nil {
		return groupmember.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return groupmember.Output{}, remoteSetupError("admin.group.member.remove", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := adminGroupMemberAdapter{adapter: connection.adapter}
	out, err := groupmember.Remove(ctx, adapter, adapter, input, preview)
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
	output, err := permissioninspect.Inspect(ctx, adminPermissionReader{connection.adapter}, input)
	return output, adminActionError("admin.permission.inspect", input.Environment, input.Site, err)
}

func adminActionError(operation, environment, site string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*errs.Error](err); ok {
		return err
	}
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact administration target and upstream response, then retry.")
	return &errs.Error{ID: operation + ".failed", Kind: errs.KindOperation, Operation: operation, Environment: environment, Site: site, Summary: "Tableau administration operation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
}

type adminGroupMemberAdapter struct{ adapter *resourceadmin.Adapter }

func (a adminGroupMemberAdapter) ResolveUsername(ctx context.Context, username string) (groupmember.Member, error) {
	user, err := a.adapter.ResolveUser(ctx, resourceadmin.UserSelector{Username: username})
	return groupmember.Member{LUID: user.LUID, Name: user.Name}, err
}

func (a adminGroupMemberAdapter) ResolveGroup(ctx context.Context, luid string) (groupmember.Group, error) {
	detail, err := a.adapter.ResolveGroup(ctx, resourceadmin.GroupSelector{LUID: luid}, true)
	members := make([]groupmember.Member, len(detail.Members))
	for i, v := range detail.Members {
		members[i] = groupmember.Member{LUID: v.LUID, Name: v.Name}
	}
	return groupmember.Group{LUID: detail.Group.LUID, Name: detail.Group.Name, Members: members}, err
}
func (a adminGroupMemberAdapter) AddGroupUser(ctx context.Context, group, user string) (groupmember.Result, error) {
	item, err := a.adapter.AddGroupUser(ctx, group, user)
	return groupmember.Result{Status: item.Status, GroupLUID: group, UserLUID: item.ResourceLUID, TableauRequestID: item.RequestID}, err
}

func (a adminGroupMemberAdapter) RemoveGroupUser(ctx context.Context, group, user string) (groupmember.Result, error) {
	item, err := a.adapter.RemoveGroupUser(ctx, group, user)
	return groupmember.Result{Status: item.Status, GroupLUID: group, UserLUID: item.ResourceLUID, TableauRequestID: item.RequestID}, err
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
