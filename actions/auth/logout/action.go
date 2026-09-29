// Package logout removes TADX-stored PAT credentials for one environment.
package logout

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

// Resolver resolves one configured environment without reading credentials.
type Resolver interface {
	Resolve(context.Context, string) (Target, error)
}

// Store removes credentials from TADX's native OS credential store.
type Store interface {
	Remove(context.Context, Target) (RemoveResult, error)
}

// Action removes one locally stored PAT without revoking it in Tableau.
type Action struct {
	resolver Resolver
	store    Store
}

// New creates auth.logout.
func New(resolver Resolver, store Store) *Action {
	return &Action{resolver: resolver, store: store}
}

// Execute removes TADX's stored credential and preserves remote PAT state.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil || a.store == nil {
		return Output{}, &errs.Error{ID: "auth.logout.unconfigured", Kind: errs.KindRuntime, Operation: "auth.logout", Summary: "PAT logout is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure environment resolution and OS credential storage before retrying."}
	}
	target, err := a.resolver.Resolve(ctx, input.Environment)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias, then retry.")
		return Output{}, &errs.Error{ID: "auth.logout.resolve", Kind: errs.KindOperation, Operation: "auth.logout", Environment: input.Environment, Summary: "Environment resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: advice, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	if input.Preview {
		var warnings []string
		if target.EnvironmentCredentialsAvailable {
			warnings = []string{"Environment-variable credentials remain configured and will continue to be used. Commands can still authenticate."}
		}
		return Output{Status: "preview", Environment: target.Environment, CredentialSource: CredentialSourceOS, Plan: &Plan{StoredCredentialReferencePresent: target.StoredCredentialReferencePresent, EnvironmentCredentialsAvailable: target.EnvironmentCredentialsAvailable}, Warnings: warnings, Help: []string{"Run without --preview to remove the selected environment's stored credential reference and credential. The Tableau PAT will not be revoked."}}, nil
	}
	removed, err := a.store.Remove(ctx, target)
	if configurationInstalled(err) {
		retryable, advice := errs.CompleteRetryAdvice(err, "Repair access to the configuration directory, then confirm the removal.")
		return Output{}, &errs.Error{ID: "auth.logout.remove", Kind: errs.KindOperation, Operation: "auth.logout", Environment: target.Environment, Summary: "The stored PAT was removed, but the configuration could not be made durable.", Cause: err, Retryable: retryable, CorrectiveAction: ensureNotRevokedAdvice(advice), Phase: errs.PhasePersistence, Outcome: errs.OutcomeConfirmed}
	}
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Repair the OS credential store, then retry. The Tableau PAT was not revoked.")
		return Output{}, &errs.Error{ID: "auth.logout.remove", Kind: errs.KindOperation, Operation: "auth.logout", Environment: target.Environment, Summary: "Stored PAT removal failed.", Cause: err, Retryable: retryable, CorrectiveAction: ensureNotRevokedAdvice(advice)}
	}
	status := "unchanged"
	if removed.Removed {
		status = "removed"
	}
	var warnings []string
	if target.EnvironmentCredentialsAvailable {
		warnings = []string{"Environment-variable credentials remain configured and will continue to be used. Commands can still authenticate."}
	}
	return Output{
		Warnings: warnings,
		Status:   status, Environment: target.Environment, CredentialSource: CredentialSourceOS, TableauPATRevoked: false,
		Help: []string{"The Tableau PAT remains valid until you revoke it in Tableau."},
	}, nil
}

// configurationInstalled reports a store failure after the configuration change
// took effect, so the stored credential was removed even though the update failed.
func configurationInstalled(err error) bool {
	var installed interface{ ConfigurationInstalled() bool }
	return errors.As(err, &installed) && installed.ConfigurationInstalled()
}

func ensureNotRevokedAdvice(value string) string {
	if strings.Contains(value, "not revoked") {
		return value
	}
	return strings.TrimSpace(value) + " The Tableau PAT was not revoked."
}

const CredentialSourceOS = "os_credential_store"

// Input selects one explicit environment credential.
type Input struct {
	Environment string
	Preview     bool
}

// Target contains nonsecret credential storage identity.
type Target struct {
	Environment                      string
	EnvironmentCredentialsAvailable  bool
	StoredCredentialReferencePresent bool
}

// RemoveResult reports whether a stored credential existed.
type RemoveResult struct {
	Removed bool
}

// Output reports local credential removal and remote PAT status separately.
type Output struct {
	Plan              *Plan    `json:"plan,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
	Status            string   `json:"status"`
	Environment       string   `json:"environment"`
	CredentialSource  string   `json:"credential_source"`
	TableauPATRevoked bool     `json:"tableau_pat_revoked"`
	Help              []string `json:"help"`
}

// Plan reports configured local removal scope without opening stored credentials.
type Plan struct {
	StoredCredentialReferencePresent bool `json:"stored_credential_reference_present"`
	EnvironmentCredentialsAvailable  bool `json:"environment_credentials_available"`
}
