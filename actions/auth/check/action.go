package check

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"

	"github.com/ahillspace/tadx/internal/errs"
)

// EnvironmentResolver resolves one exact non-secret environment profile.
type EnvironmentResolver interface {
	Resolve(context.Context, string) (Target, error)
}

// Authenticator performs PAT sign-in without exposing credentials.
type Authenticator interface {
	Authenticate(context.Context, Target) (Authentication, error)
}

// Action orchestrates auth.check.
type Action struct {
	environments  EnvironmentResolver
	authenticator Authenticator
}

// New creates auth.check.
func New(environments EnvironmentResolver, authenticator Authenticator) *Action {
	return &Action{environments: environments, authenticator: authenticator}
}

// Execute signs in and returns only non-secret identity and target context.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.environments == nil || a.authenticator == nil {
		return Output{}, &errs.Error{ID: "auth.check.unconfigured", Kind: errs.KindRuntime, Operation: "auth.check", Summary: "Authentication check is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure authentication before retrying."}
	}
	target, err := a.environments.Resolve(ctx, input.Environment)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the selected environment configuration, then retry.")
		return Output{}, &errs.Error{ID: "auth.check.resolve", Kind: errs.KindOperation, Operation: "auth.check", Environment: input.Environment, Summary: "Environment resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	if target.Environment == "" || target.ServerURL == "" {
		return Output{}, &errs.Error{ID: "auth.check.validate", Kind: errs.KindOperation, Operation: "auth.check", Environment: input.Environment, Summary: "Selected environment is incomplete.", Cause: errors.New("environment and server URL are required"), Retryable: errs.Bool(false), CorrectiveAction: "Configure the environment name and Tableau server URL before retrying."}
	}
	result, err := a.authenticator.Authenticate(ctx, target)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Verify the environment, site content URL, and PAT variable references.")
		out := Output{}
		if result.CredentialSource != "" {
			out = Output{Status: "authentication_failed", Environment: target.Environment, ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL, CredentialSource: result.CredentialSource}
		}
		failure := &errs.Error{ID: "auth.check.authenticate", Kind: errs.KindOperation, Operation: "auth.check", Environment: target.Environment, Site: target.SiteContentURL, Summary: "Authentication check failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
		if structured, ok := errors.AsType[*errs.Error](err); ok {
			failure.Phase, failure.Outcome = structured.Phase, structured.Outcome
		}
		return out, failure
	}
	return Output{Status: "authenticated", Environment: target.Environment, ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL, SiteLUID: result.SiteLUID, UserLUID: result.UserLUID, CredentialSource: result.CredentialSource, Help: []string{commandhint.Environment(target.Environment, "search", "--type", "content")}}, nil
}
