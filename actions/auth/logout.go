// Package logout removes TADX-stored PAT credentials for one environment.
package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
)

// Resolver resolves one configured environment without reading credentials.
type LogoutResolver interface {
	Resolve(context.Context, string) (LogoutTarget, error)
}

// Store removes credentials from TADX's native OS credential store.
type LogoutStore interface {
	Remove(context.Context, LogoutTarget) (LogoutRemoveResult, error)
}

// Execute removes TADX's stored credential and preserves remote PAT state.
func (a *Service) Logout(ctx context.Context, input LogoutInput) (LogoutOutput, error) {
	if a == nil || a.LogoutResolver == nil || a.LogoutStore == nil {
		return LogoutOutput{}, &errs.Error{ID: "auth.logout.unconfigured", Kind: errs.KindRuntime, Operation: "auth.logout", Summary: "PAT logout is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure environment resolution and OS credential storage before retrying."}
	}
	target, err := a.LogoutResolver.Resolve(ctx, input.Environment)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias, then retry.")
		return LogoutOutput{}, &errs.Error{ID: "auth.logout.resolve", Kind: errs.KindOperation, Operation: "auth.logout", Environment: input.Environment, Summary: "Environment resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: advice, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	if input.Preview {
		var warnings []string
		if target.EnvironmentCredentialsAvailable {
			warnings = []string{"Environment-variable credentials remain configured and will continue to be used. Commands can still authenticate."}
		}
		return LogoutOutput{Status: "preview", Environment: target.Environment, CredentialSource: CredentialSourceOS, Plan: &LogoutPlan{StoredCredentialReferencePresent: target.StoredCredentialReferencePresent, EnvironmentCredentialsAvailable: target.EnvironmentCredentialsAvailable}, Warnings: warnings, Help: []string{"Run without --preview to remove the selected environment's stored credential reference and credential. The Tableau PAT will not be revoked."}}, nil
	}
	removed, err := a.LogoutStore.Remove(ctx, target)
	if installed, ok := installedConfiguration(err); ok {
		if !installed.ExternalCommitConfirmed() {
			retryable, advice := errs.CompleteRetryAdvice(err, "Inspect the OS credential store entry and remove it if present; logout no longer references it.")
			return LogoutOutput{}, &errs.Error{ID: "auth.logout.remove", Kind: errs.KindOperation, Operation: "auth.logout", Environment: target.Environment, Summary: "The stored credential reference was cleared, but stored PAT deletion was not confirmed.", Cause: err, Retryable: retryable, CorrectiveAction: ensureNotRevokedAdvice(advice), Phase: errs.PhasePersistence, Outcome: errs.OutcomeUnknown}
		}
		retryable, advice := errs.CompleteRetryAdvice(err, "Repair access to the configuration directory, then confirm the removal.")
		return LogoutOutput{}, &errs.Error{ID: "auth.logout.remove", Kind: errs.KindOperation, Operation: "auth.logout", Environment: target.Environment, Summary: "The stored PAT was removed, but the configuration could not be made durable.", Cause: err, Retryable: retryable, CorrectiveAction: ensureNotRevokedAdvice(advice), Phase: errs.PhasePersistence, Outcome: errs.OutcomeConfirmed}
	}
	if priorConfigurationReinstalled(err) {
		retryable, _ := errs.CompleteRetryAdvice(err, "")
		advice := "Repair access to the configuration directory, then inspect the selected environment's stored reference and OS credential store entry before another logout."
		return LogoutOutput{}, &errs.Error{ID: "auth.logout.remove", Kind: errs.KindOperation, Operation: "auth.logout", Environment: target.Environment, Summary: "Stored PAT deletion was not confirmed, and the restored credential reference may not be durable.", Cause: err, Retryable: retryable, CorrectiveAction: ensureNotRevokedAdvice(advice), Phase: errs.PhasePersistence, Outcome: errs.OutcomeUnknown}
	}
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Repair the OS credential store, then retry. The Tableau PAT was not revoked.")
		return LogoutOutput{}, &errs.Error{ID: "auth.logout.remove", Kind: errs.KindOperation, Operation: "auth.logout", Environment: target.Environment, Summary: "Stored PAT removal was not confirmed.", Cause: err, Retryable: retryable, CorrectiveAction: ensureNotRevokedAdvice(advice)}
	}
	status := "unchanged"
	if removed.Removed {
		status = "removed"
	}
	var warnings []string
	if target.EnvironmentCredentialsAvailable {
		warnings = []string{"Environment-variable credentials remain configured and will continue to be used. Commands can still authenticate."}
	}
	return LogoutOutput{
		Warnings: warnings,
		Status:   status, Environment: target.Environment, CredentialSource: CredentialSourceOS, TableauPATRevoked: false,
		Help: []string{"The Tableau PAT remains valid until you revoke it in Tableau."},
	}, nil
}

func priorConfigurationReinstalled(err error) bool {
	restored, ok := errors.AsType[*config.PostSaveRestoreError](err)
	return ok && restored.PriorConfigurationReinstalled()
}

// installedConfiguration reports a store failure after the configuration change
// took effect. Credential deletion is confirmed separately, because an
// installed configuration does not prove the stored PAT was deleted.
func installedConfiguration(err error) (interface{ ExternalCommitConfirmed() bool }, bool) {
	installed, ok := errors.AsType[*config.InstalledError](err)
	if ok && installed.ConfigurationInstalled() {
		return installed, true
	}
	restored, ok := errors.AsType[*config.PostSaveRestoreError](err)
	if ok && restored.ConfigurationInstalled() {
		return restored, true
	}
	return nil, false
}

func ensureNotRevokedAdvice(value string) string {
	if strings.Contains(value, "not revoked") {
		return value
	}
	return strings.TrimSpace(value) + " The Tableau PAT was not revoked."
}

// Input selects one explicit environment credential.
type LogoutInput struct {
	Environment string
	Preview     bool
}

// Target contains nonsecret credential storage identity.
type LogoutTarget struct {
	Environment                      string
	EnvironmentCredentialsAvailable  bool
	StoredCredentialReferencePresent bool
}

// RemoveResult reports whether a stored credential existed.
type LogoutRemoveResult struct {
	Removed bool
}

// Output reports local credential removal and remote PAT status separately.
type LogoutOutput struct {
	Plan              *LogoutPlan `json:"plan,omitempty"`
	Warnings          []string    `json:"warnings,omitempty"`
	Status            string      `json:"status"`
	Environment       string      `json:"environment"`
	CredentialSource  string      `json:"credential_source"`
	TableauPATRevoked bool        `json:"tableau_pat_revoked"`
	Help              []string    `json:"help"`
}

// Plan reports configured local removal scope without opening stored credentials.
type LogoutPlan struct {
	StoredCredentialReferencePresent bool `json:"stored_credential_reference_present"`
	EnvironmentCredentialsAvailable  bool `json:"environment_credentials_available"`
}
