package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"time"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/jobmonitor"
)

// publication owns one completed write's durable receipt, never the write itself.
type publication struct {
	runtime *runtimeDependencies
	store   jobmonitor.Store
	base    jobmonitor.Receipt
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
	return &publication{runtime: r, store: jobmonitor.Store{Directory: directory}, base: jobmonitor.Receipt{ReceiptID: rand.Text(), Version: 1, Operation: kind + ".publish", Environment: c.environment.Alias, Server: c.environment.URL, Site: c.environment.SiteContentURL, SiteID: c.session.SiteLUID(), ConfigPath: r.configPath, SourcePath: filepath.ToSlash(source), ProjectID: project, Name: name}}, nil
}

func (p *publication) record(ctx context.Context, status, resourceID, requestID, verification string) (string, error) {
	r := p.base
	r.AcceptedAt = time.Now().UTC()
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
		return path, &errs.Error{ID: p.base.Operation + ".receipt", Kind: errs.KindOperation, Operation: p.base.Operation, Environment: p.base.Environment, Site: p.base.Site, Resource: resourceID, Summary: "Publication returned a result, but its recovery receipt could not be saved.", Cause: err, Phase: errs.PhasePersistence, Outcome: outcome, Retryable: errs.Bool(false), CorrectiveAction: "Preserve the returned identities. Do not repeat publication to repair local receipt storage."}
	}
	if execution := p.runtime.publicationExecution; execution != nil && execution.receiptSaved != nil {
		if err := execution.receiptSaved(saveCtx, path); err != nil {
			return filepath.ToSlash(path), fmt.Errorf("link saved publication receipt: %w", err)
		}
	}
	return filepath.ToSlash(path), nil
}
