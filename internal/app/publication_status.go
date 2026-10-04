package app

import (
	"context"
	"os"
	"path/filepath"

	jobactions "github.com/ahillspace/tadx/actions/job"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/operationrun"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourcejob "github.com/ahillspace/tadx/internal/resources/job"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableaujob "github.com/ahillspace/tadx/internal/tableau/job"
	"github.com/ahillspace/tadx/internal/value"
)

func (r *runtimeDependencies) recoveryPorts() jobactions.RecoveryPorts {
	return jobactions.RecoveryPorts{
		OperationStore:    func() (operationrun.Store, error) { return publicationOperationStore(r.operationDirectory) },
		ReceiptStore:      r.publicationReceiptStore,
		Open:              r.openRecoverySession,
		SupportsOperation: nativeLongOperation,
		ConfigPath:        r.configPath,
		Now:               r.now,
	}
}

func (r *runtimeDependencies) openRecoverySession(ctx context.Context, alias string) (jobactions.RecoverySession, error) {
	connection, err := r.tableauConnection(ctx, alias, false)
	if err != nil {
		return jobactions.RecoverySession{}, err
	}
	client := tableaujob.NewClient(connection.transport, connection.session, connection.environment.URL)
	return jobactions.RecoverySession{
		Server: connection.environment.URL, Site: connection.environment.SiteContentURL, SiteID: connection.session.SiteLUID(),
		Inspect: client.Inspect,
		ResolveDestination: func(ctx context.Context, input value.PublicationDestination) (value.ResourceDestination, error) {
			clients := r.clients(connection)
			paths := r.discoveryPaths(connection)
			ports := resourcejob.Destinations{
				Workbook:   resourceworkbook.NewAdapterWithProjectResolver(clients.workbooks, paths),
				Datasource: resourcedatasource.NewAdapterWithProjectResolver(clients.datasources, paths),
			}
			return ports.Resolve(ctx, input)
		},
	}, nil
}

func (r *runtimeDependencies) publicationReceiptStore() (jobmonitor.Store, error) {
	directory := r.jobDirectory
	if directory == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return jobmonitor.Store{}, err
		}
		directory = filepath.Join(cache, "tadx", "jobs")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return jobmonitor.Store{}, err
	}
	return jobmonitor.Store{Directory: absolute}, nil
}
