// auth.logout removes TADX-stored PAT credentials for one environment.
package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

// LogoutResolver resolves one configured environment without reading credentials.
type LogoutResolver interface {
	Resolve(context.Context, string, bool) (LogoutTarget, error)
}

// LogoutStore removes credentials from TADX's native OS credential store.
type LogoutStore interface {
	Remove(context.Context, LogoutTarget) (LogoutRemoveResult, error)
}

// LogoutAction removes one locally stored PAT without revoking it in Tableau.
type LogoutAction struct {
	resolver LogoutResolver
	store    LogoutStore
}

// NewLogout creates auth.logout.
func NewLogout(resolver LogoutResolver, store LogoutStore) *LogoutAction {
	return &LogoutAction{resolver: resolver, store: store}
}

// Execute removes TADX's stored credential and preserves remote PAT state.
func (a *LogoutAction) Execute(ctx context.Context, input LogoutInput) (LogoutOutput, error) {
	if a == nil || a.resolver == nil || a.store == nil {
		return LogoutOutput{}, &errs.Error{ID: "auth.logout.unconfigured", Kind: errs.KindRuntime, Operation: "auth.logout", Summary: "PAT logout is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure environment resolution and OS credential storage before retrying."}
	}
	target, err := a.resolver.Resolve(ctx, input.Environment, input.EnvironmentSet)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias, then retry.")
		return LogoutOutput{}, &errs.Error{ID: "auth.logout.resolve", Kind: errs.KindOperation, Operation: "auth.logout", Environment: input.Environment, Summary: "Environment resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: advice, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	var repairHelp []string
	if target.StoredCredentialReferenceInvalid {
		repairHelp = append(repairHelp, commandhint.Command("env", "remove", "--", target.Environment))
	}
	if input.Preview {
		return LogoutOutput{Status: "preview", Environment: target.Environment, CredentialSource: LogoutCredentialSourceOS, Plan: &LogoutPlan{StoredCredentialReferencePresent: target.StoredCredentialReferencePresent, EnvironmentCredentialsAvailable: target.EnvironmentCredentialsAvailable}, Warnings: target.warnings(), Help: append(repairHelp, "Run without --preview to remove the selected environment's stored credential reference and credential. The Tableau PAT will not be revoked.")}, nil
	}
	removed, err := a.store.Remove(ctx, target)
	if installed, ok := installedConfiguration(err); ok {
		if !installed.ExternalCommitConfirmed() {
			retryable, advice := errs.CompleteRetryAdvice(err, "Remove the orphaned TADX entry from the OS credential store by hand; logout no longer references it.")
			return LogoutOutput{}, &errs.Error{ID: "auth.logout.remove", Kind: errs.KindOperation, Operation: "auth.logout", Environment: target.Environment, Summary: "The stored credential reference was cleared, but the stored PAT could not be deleted.", Cause: err, Retryable: retryable, CorrectiveAction: ensureNotRevokedAdvice(advice), Phase: errs.PhasePersistence, Outcome: errs.OutcomeUnknown}
		}
		retryable, advice := errs.CompleteRetryAdvice(err, "Repair access to the configuration directory, then confirm the removal.")
		return LogoutOutput{}, &errs.Error{ID: "auth.logout.remove", Kind: errs.KindOperation, Operation: "auth.logout", Environment: target.Environment, Summary: "The stored PAT was removed, but the configuration could not be made durable.", Cause: err, Retryable: retryable, CorrectiveAction: ensureNotRevokedAdvice(advice), Phase: errs.PhasePersistence, Outcome: errs.OutcomeConfirmed}
	}
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Repair the OS credential store, then retry. The Tableau PAT was not revoked.")
		return LogoutOutput{}, &errs.Error{ID: "auth.logout.remove", Kind: errs.KindOperation, Operation: "auth.logout", Environment: target.Environment, Summary: "Stored PAT removal failed.", Cause: err, Retryable: retryable, CorrectiveAction: ensureNotRevokedAdvice(advice)}
	}
	status := "unchanged"
	if removed.Removed {
		status = "removed"
	}
	return LogoutOutput{
		Warnings: target.warnings(),
		Status:   status, Environment: target.Environment, CredentialSource: LogoutCredentialSourceOS, TableauPATRevoked: false,
		Help: append(repairHelp, "The Tableau PAT remains valid until you revoke it in Tableau."),
	}, nil
}

// installedConfiguration reports a store failure after the configuration change
// took effect. Credential deletion is confirmed separately, because an
// installed configuration does not prove the stored PAT was deleted.
func installedConfiguration(err error) (interface{ ExternalCommitConfirmed() bool }, bool) {
	var installed interface {
		ConfigurationInstalled() bool
		ExternalCommitConfirmed() bool
	}
	if errors.As(err, &installed) && installed.ConfigurationInstalled() {
		return installed, true
	}
	return nil, false
}

func ensureNotRevokedAdvice(value string) string {
	if strings.Contains(value, "not revoked") {
		return value
	}
	return strings.TrimSpace(value) + " The Tableau PAT was not revoked."
}

// LogoutCredentialSourceOS names TADX's native credential source.
const LogoutCredentialSourceOS = "os_credential_store"

// LogoutInput selects one explicit environment credential.
type LogoutInput struct {
	Environment    string
	EnvironmentSet bool `json:"-"`
	Preview        bool
}

// LogoutTarget contains nonsecret credential storage identity.
type LogoutTarget struct {
	Environment                      string
	EnvironmentCredentialsAvailable  bool
	StoredCredentialReferencePresent bool
	StoredCredentialReferenceInvalid bool
}

func (target LogoutTarget) warnings() []string {
	var warnings []string
	if target.StoredCredentialReferenceInvalid {
		warnings = append(warnings, "No stored credential can be located because credential_ref is invalid. Review its settings before removing this environment.")
	}
	if target.EnvironmentCredentialsAvailable {
		warnings = append(warnings, "Environment-variable credentials remain configured and will continue to be used. Commands can still authenticate.")
	}
	return warnings
}

// LogoutRemoveResult reports whether a stored credential existed.
type LogoutRemoveResult struct {
	Removed bool
}

// LogoutOutput reports local credential removal and remote PAT status separately.
type LogoutOutput struct {
	Plan              *LogoutPlan `json:"plan,omitempty"`
	Warnings          []string    `json:"warnings,omitempty"`
	Status            string      `json:"status"`
	Environment       string      `json:"environment"`
	CredentialSource  string      `json:"credential_source"`
	TableauPATRevoked bool        `json:"tableau_pat_revoked"`
	Help              []string    `json:"help"`
}

// LogoutPlan reports configured local removal scope without opening stored credentials.
type LogoutPlan struct {
	StoredCredentialReferencePresent bool `json:"stored_credential_reference_present"`
	EnvironmentCredentialsAvailable  bool `json:"environment_credentials_available"`
}
