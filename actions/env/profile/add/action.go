package add

import (
	"context"
	"github.com/ahillspace/tadx/internal/commandhint"
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
	if input.CacheMaxConcurrency < 0 || input.CacheMaxConcurrency > 256 {
		return Output{}, usageError("cache maximum concurrency must be between 1 and 256, or omitted for the default")
	}
	add := a.adder.Add
	if input.Preview {
		previewer, ok := a.adder.(interface {
			PreviewAdd(context.Context, Profile) (Profile, error)
		})
		if !ok {
			return Output{}, &errs.Error{ID: "env.profile.add.preview", Kind: errs.KindRuntime, Operation: "env.profile.add", Summary: "Profile preview is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure a read-only profile preview store."}
		}
		add = previewer.PreviewAdd
	}
	profile, err := add(ctx, Profile{Alias: input.Alias, ServerURL: input.ServerURL, SiteContentURL: input.SiteContentURL, APIVersion: input.APIVersion, AuthType: "pat", PATNameEnv: input.PATNameEnv, PATSecretEnv: input.PATSecretEnv, DefaultWorkspace: input.DefaultWorkspace, CacheMaxConcurrency: input.CacheMaxConcurrency})
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the new environment profile and alias, then retry.")
		id, summary := "env.profile.add.write", "Environment profile could not be added."
		if input.Preview {
			id, summary = "env.profile.add.preview", "Environment profile preview failed."
		}
		return Output{}, &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "env.profile.add", Environment: input.Alias, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	var warnings []string
	if profile.MultipleEnvironments {
		warnings = []string{"Multiple environments are now configured. Remote writes require --env <name>. Reads still use your configured default."}
	}
	if input.Preview {
		return Output{Status: "preview", Profile: profile, Help: []string{"Execution adds this profile after rechecking the configuration. The configuration has not been saved."}}, nil
	}
	return Output{Warnings: warnings, Status: "added", Profile: profile, Help: []string{commandhint.Environment(profile.Alias, "auth", "status")}}, nil
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
