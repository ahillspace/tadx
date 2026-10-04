package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/output"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
	tableaujob "github.com/ahillspace/tadx/internal/tableau/job"
)

// publication owns one accepted write's durable identity, never the write itself.
type publication struct {
	runtime    *runtimeDependencies
	connection authenticatedTableau
	*jobmonitor.Publication
	asJob bool
}

func authTarget(c authenticatedTableau) coreauth.Target {
	e := c.environment
	return coreauth.Target{Environment: e.Alias, ServerURL: e.URL, SiteContentURL: e.SiteContentURL, PATNameVariable: e.Auth.PATNameEnv, PATSecretVariable: e.Auth.PATSecretEnv, CredentialReference: e.Auth.CredentialRef}
}

func (r *runtimeDependencies) publication(ctx context.Context, environment, kind, source, project, name string) (*publication, error) {
	c, err := r.tableauConnection(ctx, environment, true)
	if err != nil {
		return nil, err
	}
	directory := r.jobDirectory
	if directory == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		directory = filepath.Join(cache, "tadx", "jobs")
	}
	// Query Job requires administrator permissions. Automatically retain the
	// synchronous contract where that monitoring prerequisite is unavailable.
	key, err := r.commandSessions().CoordinationKey(ctx, authTarget(c))
	if err != nil {
		return nil, err
	}
	async := false
	// The role read only selects optional job monitoring. A denied inspection
	// preserves synchronous publication without making the administrative read.
	if kind != "flow" && r.checkManagedCapability("admin.user.inspect") == nil {
		r.command.mu.Lock()
		known, present := r.command.jobMonitoring[key]
		r.command.mu.Unlock()
		if !present {
			user, roleErr := tableauadmin.NewClient(c.transport, c.session, c.environment.URL).GetUser(ctx, c.session.UserLUID())
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			known = roleErr == nil && (user.SiteRole == "ServerAdministrator" || user.SiteRole == "SiteAdministratorExplorer" || user.SiteRole == "SiteAdministratorCreator")
			r.command.mu.Lock()
			if r.command.jobMonitoring == nil {
				r.command.jobMonitoring = make(map[string]bool)
			}
			r.command.jobMonitoring[key] = known
			r.command.mu.Unlock()
		}
		async = known
	}
	p := &publication{runtime: r, connection: c, asJob: async}
	p.Publication = &jobmonitor.Publication{
		Store:         jobmonitor.Store{Directory: directory},
		Base:          jobmonitor.Receipt{ReceiptID: rand.Text(), Version: 1, Operation: kind + ".publish", Environment: c.environment.Alias, Server: c.environment.URL, Site: c.environment.SiteContentURL, SiteID: c.session.SiteLUID(), ConfigPath: r.configPath, SourcePath: filepath.ToSlash(source), ProjectID: project, Name: name, CoordinationKey: key},
		Deadline:      r.publicationWaitDeadline,
		StopRequested: r.publicationNoWait,
		OnAccepted: func(ctx context.Context, path string) error {
			if execution := r.publicationExecution; execution != nil && execution.accepted != nil {
				return execution.accepted(ctx, path)
			}
			return nil
		},
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
		Observe:      p.observe,
	}
	if execution := r.publicationExecution; execution != nil && execution.prepare != nil {
		p.Base.OperationID = execution.operationID
		if err := execution.prepare(ctx, p.Base); err != nil {
			return nil, fmt.Errorf("record publication receipt intent before submission: %w", err)
		}
	}
	return p, nil
}

func (p *publication) observe(ctx context.Context, jobs []jobmonitor.Receipt) []jobmonitor.CheckResult {
	results := make([]jobmonitor.CheckResult, len(jobs))
	sessions := p.runtime.commandSessions()
	for i, job := range jobs {
		session, err := sessions.AuthenticateMonitor(ctx, authTarget(p.connection), tableauauth.NewClient(p.connection.transport))
		if err != nil {
			results[i].Err = errors.Join(err, sessions.Suspend(context.WithoutCancel(ctx)))
			continue
		}
		client := tableaujob.NewClient(p.connection.transport, session, p.connection.environment.URL)
		if normalizeServer(job.Server) != normalizeServer(p.Base.Server) || job.SiteID != session.SiteLUID() {
			results[i].Err = errors.New("job monitoring target does not match the authenticated site")
		} else {
			results[i].Status, results[i].Err = client.Inspect(ctx, job.Observation.ID)
			if results[i].Err == nil && job.Observation.Type != "" && results[i].Status.Type != job.Observation.Type {
				results[i].Err = errors.New("job observation changed the accepted job type")
			}
		}
		// Foreground requests may enter between individual exact observations.
		results[i].Err = errors.Join(results[i].Err, sessions.Suspend(context.WithoutCancel(ctx)))
	}
	return results
}

func publicationError(operation, environment, site, id string, status string, cause error) error {
	if cause == nil {
		return nil
	}
	if known, ok := errors.AsType[*errs.Error](cause); ok && known.Phase == errs.PhasePersistence {
		return cause
	}
	state := errs.OutcomeUnknown
	summary := "Publication was accepted, but local monitoring did not establish its final outcome."
	if status == "succeeded" {
		state, summary = errs.OutcomeConfirmed, "Publication succeeded, but destination confirmation is incomplete."
	}
	if status == "failed" || status == "cancelled" {
		summary = "Tableau reports that publication " + status + "."
	}
	return &errs.Error{ID: operation + ".monitor", Kind: errs.KindOperation, Operation: operation, Environment: environment, Site: site, Summary: summary, Cause: cause, Phase: errs.PhaseVerification, Outcome: state, TableauJobID: id, Retryable: errs.Bool(false), CorrectiveAction: "Recover status using the saved job identity. Do not repeat publication to recover its outcome."}
}
