package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	jobactions "github.com/ahillspace/tadx/actions/job"
	jobcli "github.com/ahillspace/tadx/internal/cli/job"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
	tableaujob "github.com/ahillspace/tadx/internal/tableau/job"
)

type jobProvider struct{ runtime *runtimeDependencies }

func newJobDependencies(runtime *runtimeDependencies) *jobcli.Dependencies {
	service := jobactions.New(jobProvider{runtime: runtime})
	return &jobcli.Dependencies{
		Inspect: service.Inspect, Wait: service.Wait, Cancel: service.Cancel,
		InspectUse: registryLeafUse("job.inspect"), InspectShort: registryShort("job.inspect"),
		WaitUse: registryLeafUse("job.wait"), WaitShort: registryShort("job.wait"),
		CancelUse: registryLeafUse("job.cancel"), CancelShort: registryShort("job.cancel"),
	}
}

func (p jobProvider) Open(ctx context.Context, alias string) (jobactions.Session, error) {
	connection, err := p.runtime.tableauConnection(ctx, alias, false)
	result := jobactions.Session{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, Server: connection.environment.URL, ConfigPath: p.runtime.configPath}
	if err != nil {
		return result, err
	}
	result.SiteID = connection.session.SiteLUID()
	result.Native = tableaujob.NewClient(connection.transport, connection.session, connection.environment.URL)
	result.CoordinationKey = func(ctx context.Context) (string, error) {
		return p.runtime.commandSessions().CoordinationKey(ctx, authTarget(connection))
	}
	observer := p.runtime.jobObserver(connection, connection.environment.URL, "job receipt target does not match the authenticated site")
	result.Observe = observer.Observe
	return result, nil
}

func (p jobProvider) Suspend(ctx context.Context) error {
	return p.runtime.commandSessions().Suspend(ctx)
}
func (p jobProvider) RecoveryPorts() jobactions.RecoveryPorts { return p.runtime.recoveryPorts() }

func (c jobProvider) Store() (jobmonitor.Store, error) {
	directory := c.runtime.jobDirectory
	if directory == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return jobmonitor.Store{}, errors.New("job recovery directory is unavailable")
		}
		directory = filepath.Join(cache, "tadx", "jobs")
	}
	return jobmonitor.Store{Directory: directory}, nil
}

func (r *runtimeDependencies) jobObserver(connection authenticatedTableau, server, targetError string) jobmonitor.Observer {
	sessions := r.commandSessions()
	return jobmonitor.Observer{
		TargetError: targetError,
		Open: func(ctx context.Context) (jobmonitor.ObservationSession, error) {
			session, err := sessions.AuthenticateMonitor(ctx, authTarget(connection), tableauauth.NewClient(connection.transport))
			if err != nil {
				return jobmonitor.ObservationSession{}, err
			}
			client := tableaujob.NewClient(connection.transport, session, connection.environment.URL)
			return jobmonitor.ObservationSession{Server: server, SiteID: session.SiteLUID(), Inspect: client.Inspect}, nil
		},
		Suspend: sessions.Suspend,
	}
}
