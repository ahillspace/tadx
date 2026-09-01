package add

import (
	"context"
	"net/url"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type Adder interface {
	Add(context.Context, Profile) (Profile, error)
}

type Action struct{ adder Adder }

func New(adder Adder) *Action { return &Action{adder: adder} }

func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.adder == nil {
		return Output{}, &errs.Error{ID: "env.profile.add.unconfigured", Kind: errs.KindRuntime, Operation: "env.profile.add", Summary: "Environment profile creation is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the environment profile store before retrying."}
	}
	if strings.TrimSpace(input.Alias) == "" {
		return Output{}, usageError("environment alias is required")
	}
	if err := validateServerURL(input.ServerURL); err != nil {
		return Output{}, usageError(err.Error())
	}
	if input.PATNameEnv != "" && strings.EqualFold(input.PATNameEnv, input.PATSecretEnv) {
		return Output{}, usageError("PAT name and secret must use different environment variables")
	}
	profile, err := a.adder.Add(ctx, Profile{Alias: input.Alias, ServerURL: input.ServerURL, SiteContentURL: input.SiteContentURL, APIVersion: input.APIVersion, AuthType: "pat", PATNameEnv: input.PATNameEnv, PATSecretEnv: input.PATSecretEnv, DefaultWorkspace: input.DefaultWorkspace})
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the new environment profile and alias, then retry.")
		return Output{}, &errs.Error{ID: "env.profile.add.write", Kind: errs.KindOperation, Operation: "env.profile.add", Environment: input.Alias, Summary: "Environment profile could not be added.", Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	return Output{Status: "added", Profile: profile, Help: []string{"tadx auth status --environment <alias>"}}, nil
}

func validateServerURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return &validationError{"server URL must be an absolute HTTPS URL without credentials, query, or fragment"}
	}
	return nil
}

type validationError struct{ message string }

func (e *validationError) Error() string { return e.message }
func usageError(summary string) error {
	return &errs.Error{ID: "env.profile.add.usage", Kind: errs.KindUsage, Operation: "env.profile.add", Summary: summary}
}
