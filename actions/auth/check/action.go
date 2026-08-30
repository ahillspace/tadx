package check

import (
	"context"
	"errors"

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
		return Output{}, errs.New(errs.KindRuntime, "Authentication check is not configured.")
	}
	target, err := a.environments.Resolve(ctx, input.Environment)
	if err != nil {
		return Output{}, &errs.Error{ID: "auth.environment.resolve", Kind: errs.KindOperation, Operation: "auth.check", Environment: input.Environment, Summary: "Environment resolution failed.", Cause: err}
	}
	if target.Environment == "" || target.ServerURL == "" {
		return Output{}, &errs.Error{ID: "auth.environment.invalid", Kind: errs.KindOperation, Operation: "auth.check", Environment: input.Environment, Summary: "Selected environment is incomplete.", Cause: errors.New("environment and server URL are required")}
	}
	result, err := a.authenticator.Authenticate(ctx, target)
	if err != nil {
		return Output{}, &errs.Error{ID: "auth.check.failed", Kind: errs.KindOperation, Operation: "auth.check", Environment: target.Environment, Site: target.SiteContentURL, Summary: "Authentication check failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Verify the environment, site content URL, and PAT variable references.", TableauRequestID: errs.TableauRequestID(err)}
	}
	return Output{Status: "authenticated", Environment: target.Environment, ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL, SiteLUID: result.SiteLUID, UserLUID: result.UserLUID}, nil
}
