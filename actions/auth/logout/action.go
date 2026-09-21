package logout

import (
	"context"
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
	if strings.TrimSpace(input.Environment) == "" {
		return Output{}, &errs.Error{ID: "auth.logout.usage", Kind: errs.KindUsage, Operation: "auth.logout", Summary: "--environment is required", Retryable: errs.Bool(false), CorrectiveAction: "Provide the exact environment whose stored PAT should be removed."}
	}
	target, err := a.resolver.Resolve(ctx, input.Environment)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias, then retry.")
		return Output{}, &errs.Error{ID: "auth.logout.resolve", Kind: errs.KindOperation, Operation: "auth.logout", Environment: input.Environment, Summary: "Environment resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: advice, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	if strings.TrimSpace(target.Environment) == "" {
		return Output{}, &errs.Error{ID: "auth.logout.target", Kind: errs.KindOperation, Operation: "auth.logout", Environment: input.Environment, Summary: "Selected environment is incomplete.", Retryable: errs.Bool(false), CorrectiveAction: "Complete the selected environment profile before retrying."}
	}
	if input.Preview {
		var warnings []string
		if target.EnvironmentCredentialsAvailable {
			warnings = []string{"Environment-variable credentials remain configured and will continue to be used. Commands can still authenticate."}
		}
		return Output{Status: "preview", Environment: target.Environment, CredentialSource: CredentialSourceOS, Plan: &Plan{StoredCredentialReferencePresent: target.StoredCredentialReferencePresent, EnvironmentCredentialsAvailable: target.EnvironmentCredentialsAvailable}, Warnings: warnings, Help: []string{"Run without --preview to remove the selected environment's stored credential reference and credential. The Tableau PAT will not be revoked."}}, nil
	}
	removed, err := a.store.Remove(ctx, target)
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

func ensureNotRevokedAdvice(value string) string {
	if strings.Contains(value, "not revoked") {
		return value
	}
	return strings.TrimSpace(value) + " The Tableau PAT was not revoked."
}
