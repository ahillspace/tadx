package app

import (
	"context"

	permission "github.com/ahillspace/tadx/actions/admin/permission"
	resourceadmin "github.com/ahillspace/tadx/internal/resources/admin"
)

type adminPermissionProvider struct{ commands *remoteAdminCommands }

func (p adminPermissionProvider) Open(ctx context.Context, alias, site, operation string, explicit bool) (permission.LiveSession, error) {
	connection, err := p.commands.connect(ctx, alias, explicit)
	if err != nil {
		return permission.LiveSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	return permission.LiveSession{Target: permission.Target{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL}, Ports: resourceadmin.PermissionPorts{Adapter: connection.adapter}}, nil
}
