package app

import (
	"context"
	"os"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/output"
	resourcejob "github.com/ahillspace/tadx/internal/resources/job"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
)

func authTarget(c authenticatedTableau) coreauth.Target {
	e := c.environment
	return coreauth.Target{Environment: e.Alias, ServerURL: e.URL, SiteContentURL: e.SiteContentURL, PATNameVariable: e.Auth.PATNameEnv, PATSecretVariable: e.Auth.PATSecretEnv, CredentialReference: e.Auth.CredentialRef}
}

func (r *runtimeDependencies) publication(ctx context.Context, environment, kind, source, project, name string) (*jobmonitor.Publication, bool, error) {
	c, err := r.tableauConnection(ctx, environment, true)
	if err != nil {
		return nil, false, err
	}
	store, err := jobmonitor.NewStore(r.jobDirectory)
	if err != nil {
		return nil, false, err
	}
	// Query Job requires administrator permissions. Automatically retain the
	// synchronous contract where that monitoring prerequisite is unavailable.
	key, err := r.commandSessions().CoordinationKey(ctx, authTarget(c))
	if err != nil {
		return nil, false, err
	}
	async, err := r.command.jobMonitoring.CanMonitor(ctx, kind, key,
		func() bool { return r.checkManagedCapability("admin.user.inspect") == nil },
		func(ctx context.Context) (bool, error) {
			port := resourcejob.MonitoringRolePort{Reader: tableauadmin.NewClient(c.transport, c.session, c.environment.URL), UserLUID: c.session.UserLUID()}
			return port.Eligible(ctx)
		})
	if err != nil {
		return nil, false, err
	}
	var operationID string
	var prepare func(context.Context, jobmonitor.Receipt) error
	if execution := r.publicationExecution; execution != nil {
		if execution.HasPrepareIntent() {
			operationID = execution.OperationID
			prepare = func(ctx context.Context, receipt jobmonitor.Receipt) error {
				return execution.RecordIntent(ctx, receipt.Operation, receipt.ReceiptID, jobmonitor.ReceiptScope(receipt))
			}
		}
	}
	monitor, err := jobmonitor.StartPublication(ctx, store, jobmonitor.PublicationTarget{
		Kind: kind, Environment: c.environment.Alias, Server: c.environment.URL,
		Site: c.environment.SiteContentURL, SiteID: c.session.SiteLUID(),
		ConfigPath: r.configPath, SourcePath: source, ProjectID: project, Name: name,
		CoordinationKey: key, OperationID: operationID,
	}, jobmonitor.PublicationHooks{
		Deadline:      r.publicationExecution.WaitDeadline,
		StopRequested: r.publicationExecution.StopRequested,
		Prepare:       prepare,
		OnAccepted:    r.publicationExecution.LinkAccepted,
		NotifyAccepted: func(id, path string) {
			writer := r.progressWriter
			if writer == nil {
				writer = os.Stderr
			}
			_ = output.RenderWithOptions(writer, struct {
				Status      string `json:"status"`
				JobID       string `json:"tableau_job_id"`
				ReceiptPath string `json:"receipt_path"`
			}{"accepted", id, path}, output.Options{})
		},
		StartWaiting: func(ctx context.Context) { progress.SetLabel(ctx, "Waiting for Tableau publication") },
		Suspend:      func(ctx context.Context) error { return r.commandSessions().Suspend(ctx) },
		Observe:      r.jobObserver(c, c.environment.URL, "job monitoring target does not match the authenticated site").Observe,
	})
	if err != nil {
		return nil, false, err
	}
	return monitor, async, nil
}
