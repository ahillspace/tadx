// Package auth implements PAT authentication actions.
package auth

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

// CheckEnvironmentResolver resolves one exact non-secret environment profile.
type CheckEnvironmentResolver interface {
	Resolve(context.Context, string) (CheckTarget, error)
}

// CheckAuthenticator performs PAT sign-in without exposing credentials.
type CheckAuthenticator interface {
	Authenticate(context.Context, CheckTarget) (CheckAuthentication, error)
}

// CheckAction orchestrates auth.check.
type CheckAction struct {
	environments  CheckEnvironmentResolver
	authenticator CheckAuthenticator
}

// NewCheck creates auth.check.
func NewCheck(environments CheckEnvironmentResolver, authenticator CheckAuthenticator) *CheckAction {
	return &CheckAction{environments: environments, authenticator: authenticator}
}

// Execute signs in and returns only non-secret identity and target context.
func (a *CheckAction) Execute(ctx context.Context, input CheckInput) (CheckOutput, error) {
	if a == nil || a.environments == nil || a.authenticator == nil {
		return CheckOutput{}, &errs.Error{ID: "auth.check.unconfigured", Kind: errs.KindRuntime, Operation: "auth.check", Summary: "Authentication check is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure authentication before retrying."}
	}
	target, err := a.environments.Resolve(ctx, input.Environment)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the selected environment configuration, then retry.")
		return CheckOutput{}, &errs.Error{ID: "auth.check.resolve", Kind: errs.KindOperation, Operation: "auth.check", Environment: input.Environment, Summary: "Environment resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	result, err := a.authenticator.Authenticate(ctx, target)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Verify the environment, site content URL, and PAT variable references.")
		out := CheckOutput{}
		if result.CredentialSource != "" {
			out = CheckOutput{Status: "authentication_failed", Environment: target.Environment, ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL, CredentialSource: result.CredentialSource}
		}
		failure := &errs.Error{ID: "auth.check.authenticate", Kind: errs.KindOperation, Operation: "auth.check", Environment: target.Environment, Site: target.SiteContentURL, Summary: "Authentication check failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
		if structured, ok := errors.AsType[*errs.Error](err); ok {
			failure.Phase, failure.Outcome = structured.Phase, structured.Outcome
		}
		return out, failure
	}
	return CheckOutput{Status: "authenticated", Environment: target.Environment, ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL, SiteLUID: result.SiteLUID, UserLUID: result.UserLUID, CredentialSource: result.CredentialSource, RESTAPIVersion: result.RESTAPIVersion, Help: []string{commandhint.Environment(target.Environment, "search", "--type", "content")}}, nil
}

// CheckInput selects one configured Tableau environment.
type CheckInput struct {
	Environment string
}

// CheckTarget contains non-secret sign-in context.
type CheckTarget struct {
	Environment         string
	ServerURL           string
	SiteContentURL      string
	PATNameVariable     string
	PATSecretVariable   string
	CredentialReference string
}

// CheckAuthentication is the non-secret result of PAT sign-in.
type CheckAuthentication struct {
	SiteLUID         string
	UserLUID         string
	CredentialSource string
	RESTAPIVersion   string
}

// CheckOutput is the stable authenticated target result.
type CheckOutput struct {
	Status           string   `json:"status"`
	Environment      string   `json:"environment"`
	ServerURL        string   `json:"server_url"`
	SiteContentURL   string   `json:"site_content_url"`
	SiteLUID         string   `json:"site_luid,omitempty"`
	UserLUID         string   `json:"user_luid,omitempty"`
	CredentialSource string   `json:"credential_source,omitempty"`
	RESTAPIVersion   string   `json:"rest_api_version,omitempty"`
	Help             []string `json:"help"`
}
