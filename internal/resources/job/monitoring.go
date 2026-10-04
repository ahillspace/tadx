package job

import (
	"context"

	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
)

type MonitorRoleReader interface {
	GetUser(context.Context, string) (tableauadmin.User, error)
}

// MonitoringRolePort reads native site administration status only when policy permits it.
type MonitoringRolePort struct {
	Reader   MonitorRoleReader
	UserLUID string
}

func (p MonitoringRolePort) Eligible(ctx context.Context) (bool, error) {
	user, err := p.Reader.GetUser(ctx, p.UserLUID)
	if err != nil {
		return false, err
	}
	switch user.SiteRole {
	case "ServerAdministrator", "SiteAdministratorExplorer", "SiteAdministratorCreator":
		return true, nil
	default:
		return false, nil
	}
}
