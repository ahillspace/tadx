package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/output"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
	tableaujob "github.com/ahillspace/tadx/internal/tableau/job"
	"github.com/ahillspace/tadx/internal/value"
)

// publication owns one accepted write's durable identity, never the write itself.
type publication struct {
	runtime    *runtimeDependencies
	connection authenticatedTableau
	store      jobmonitor.Store
	base       jobmonitor.Receipt
	asJob      bool
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
	if kind != "flow" {
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
	return &publication{runtime: r, connection: c, store: jobmonitor.Store{Directory: directory}, asJob: async, base: jobmonitor.Receipt{ReceiptID: rand.Text(), Version: 1, Operation: kind + ".publish", Environment: c.environment.Alias, Server: c.environment.URL, Site: c.environment.SiteContentURL, SiteID: c.session.SiteLUID(), ConfigPath: r.configPath, SourcePath: filepath.ToSlash(source), ProjectID: project, Name: name, CoordinationKey: key}}, nil
}

func (p *publication) record(ctx context.Context, jobID, status, resourceID, requestID, verification string) (string, error) {
	r := p.base
	r.Observation.ID = jobID
	if jobID != "" {
		if saved, err := p.store.Read(r); err == nil {
			r = saved
		}
	}
	if r.AcceptedAt.IsZero() {
		r.AcceptedAt = time.Now().UTC()
	}
	r.Observation.Status, r.Observation.ResourceID = status, resourceID
	if requestID != "" {
		r.Observation.RequestID = requestID
	}
	r.Verification = verification
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	path, err := p.store.Save(saveCtx, r)
	if err != nil {
		outcome := errs.OutcomeUnknown
		if status == "succeeded" {
			outcome = errs.OutcomeConfirmed
		}
		return path, &errs.Error{ID: p.base.Operation + ".receipt", Kind: errs.KindOperation, Operation: p.base.Operation, Environment: p.base.Environment, Site: p.base.Site, Resource: resourceID, TableauJobID: jobID, Summary: "Publication returned a result, but its recovery receipt could not be saved.", Cause: err, Phase: errs.PhasePersistence, Outcome: outcome, Retryable: errs.Bool(false), CorrectiveAction: "Preserve the returned identities. Do not repeat publication to repair local receipt storage."}
	}
	return filepath.ToSlash(path), nil
}

func (p *publication) accepted(ctx context.Context, id, requestID, jobType string) (jobmonitor.Receipt, string, error) {
	r := p.base
	r.AcceptedAt = time.Now().UTC()
	r.PoolAfter = r.AcceptedAt.Add(time.Minute)
	if contentbatch.Bulk(ctx) {
		r.PoolAfter = r.AcceptedAt
	}
	r.Observation = value.JobStatus{ID: id, Type: jobType, Status: "pending", RequestID: requestID}
	// Acceptance must survive cancellation that arrives just after the server's
	// response. This local persistence deadline never resubmits a remote write.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	path, err := p.store.Register(saveCtx, r)
	if err != nil {
		return r, path, err
	}
	writer := p.runtime.progressWriter
	if writer == nil {
		writer = os.Stderr
	}
	_ = output.RenderWithOptions(writer, struct {
		Status      string `json:"status"`
		JobID       string `json:"tableau_job_id"`
		ReceiptPath string `json:"receipt_path"`
	}{"accepted", id, filepath.ToSlash(path)}, output.Options{})
	if contentbatch.Bulk(ctx) {
		return r, path, nil
	}
	observed, err := p.wait(ctx, r)
	return observed, path, err
}

func (p *publication) wait(ctx context.Context, r jobmonitor.Receipt) (jobmonitor.Receipt, error) {
	if r.AcceptedAt.IsZero() {
		stored, err := p.store.Read(r)
		if err != nil {
			return r, err
		}
		r = stored
	}
	if err := p.runtime.commandSessions().Suspend(ctx); err != nil {
		return r, err
	}
	m := jobmonitor.Monitor{Store: p.store, Observe: func(ctx context.Context, jobs []jobmonitor.Receipt) []jobmonitor.CheckResult {
		results := make([]jobmonitor.CheckResult, len(jobs))
		sessions := p.runtime.commandSessions()
		for i, job := range jobs {
			session, err := sessions.AuthenticateMonitor(ctx, authTarget(p.connection), tableauauth.NewClient(p.connection.transport))
			if err != nil {
				results[i].Err = errors.Join(err, sessions.Suspend(context.WithoutCancel(ctx)))
				continue
			}
			client := tableaujob.NewClient(p.connection.transport, session, p.connection.environment.URL)
			if normalizeServer(job.Server) != normalizeServer(p.base.Server) || job.SiteID != session.SiteLUID() {
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
	}}
	observed, err := m.Wait(ctx, r)
	if err == nil && observed.Observation.Status != "succeeded" {
		err = fmt.Errorf("remote job is %s", observed.Observation.Status)
	}
	return observed, err
}

func publicationError(operation, environment, site, id string, status string, cause error) error {
	if cause == nil {
		return nil
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
