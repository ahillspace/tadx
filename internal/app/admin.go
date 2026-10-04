package app

import (
	"context"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	permission "github.com/ahillspace/tadx/actions/admin/permission"
	userops "github.com/ahillspace/tadx/actions/admin/user"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/inventory"
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
	users := userops.New(adminUserProvider{commands: c})
	groups := groupops.New(adminGroupProvider{commands: c})
	permissions := permission.New(adminPermissionProvider{commands: c})
	return &admincli.Dependencies{
		PermissionCapabilities: tableauadmin.PermissionCapabilities,
		UserLister:             users, UserInspector: users, UserCreator: users, UserUpdater: users, UserDeleter: users,
		GroupLister: groups, GroupInspector: groups, GroupCreator: groups, GroupUpdater: groups, GroupDeleter: groups,
		GroupMemberAdder: groups, GroupMemberRemover: groups,
		PermissionInspector: permissions, PermissionCreator: permissions, PermissionDeleter: permissions,
	}
}

type adminConnection struct {
	environment config.Environment
	adapter     *resourceadmin.Adapter
	inventory   tableaucache.Executor
	callerLUID  string
}

func (c *remoteAdminCommands) connect(ctx context.Context, alias string, explicit bool) (adminConnection, error) {
	connection, err := c.runtime.tableauConnection(ctx, alias, explicit)
	if err != nil {
		return adminConnection{environment: connection.environment}, err
	}
	client := tableauadmin.NewClient(connection.transport, connection.session, connection.environment.URL)
	native := tableaucache.AuthenticatedExecutor{Transport: connection.transport, Session: connection.session, ServerURL: connection.environment.URL, SiteLUID: connection.session.SiteLUID()}
	return adminConnection{environment: connection.environment, adapter: resourceadmin.NewAdapter(client, c.runtime.checkManagedCapability), inventory: inventory.AuthorizedExecutor{Next: native, CheckScope: c.runtime.checkManagedCapability}, callerLUID: connection.session.UserLUID()}, nil
}

var _ admincli.UserLister = (*userops.Service)(nil)
var _ admincli.UserInspector = (*userops.Service)(nil)
var _ admincli.UserCreator = (*userops.Service)(nil)
var _ admincli.UserUpdater = (*userops.Service)(nil)
var _ admincli.UserDeleter = (*userops.Service)(nil)
var _ admincli.GroupLister = (*groupops.Service)(nil)
var _ admincli.GroupInspector = (*groupops.Service)(nil)
var _ admincli.GroupCreator = (*groupops.Service)(nil)
var _ admincli.GroupUpdater = (*groupops.Service)(nil)
var _ admincli.GroupDeleter = (*groupops.Service)(nil)
var _ admincli.GroupMemberAdder = (*groupops.Service)(nil)
var _ admincli.GroupMemberRemover = (*groupops.Service)(nil)
var _ admincli.PermissionInspector = (*permission.Service)(nil)
var _ admincli.PermissionCreator = (*permission.Service)(nil)
var _ admincli.PermissionDeleter = (*permission.Service)(nil)
