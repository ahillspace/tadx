package update

import (
	"context"
	"net/url"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type Updater interface {
	Update(context.Context, string, Patch) (UpdateResult, error)
}

type Action struct{ updater Updater }

func New(updater Updater) *Action { return &Action{updater: updater} }

func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.updater == nil {
		return Output{}, &errs.Error{ID: "env.profile.update.unconfigured", Kind: errs.KindRuntime, Operation: "env.profile.update", Summary: "Environment profile update is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the environment profile store before retrying."}
	}
	if strings.TrimSpace(input.Alias) == "" {
		return Output{}, usageError("environment alias is required")
	}
	if !input.Patch.Any() {
		return Output{}, usageError("at least one profile field must be supplied")
	}
	if value := input.Patch.CatalogMaxConcurrency; value.Set && (value.Value < 0 || value.Value > 256) {
		return Output{}, usageError("catalog maximum concurrency must be between 1 and 256, or cleared for the default")
	}
	if input.Patch.ServerURL.Set && !validServerURL(input.Patch.ServerURL.Value) {
		return Output{}, usageError("server URL must be an absolute HTTPS URL without credentials, query, or fragment")
	}
	if input.Patch.PATNameEnv.Set && input.Patch.PATSecretEnv.Set && input.Patch.PATNameEnv.Value != "" && strings.EqualFold(input.Patch.PATNameEnv.Value, input.Patch.PATSecretEnv.Value) {
		return Output{}, usageError("PAT name and secret must use different environment variables")
	}
	result, err := a.updater.Update(ctx, input.Alias, input.Patch)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias and requested fields, then retry.")
		return Output{}, &errs.Error{ID: "env.profile.update.write", Kind: errs.KindOperation, Operation: "env.profile.update", Environment: input.Alias, Summary: "Environment profile could not be updated.", Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	changedFields := normalizeChangedFields(result.ChangedFields)
	status := "unchanged"
	if len(changedFields) > 0 {
		status = "updated"
	}
	return Output{Status: status, Profile: result.Profile, ChangedFields: changedFields, Help: []string{"tadx env get <alias>"}}, nil
}

func validServerURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && strings.EqualFold(parsed.Scheme, "https") && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func normalizeChangedFields(fields []string) []string {
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if field != "" {
			seen[field] = true
		}
	}
	result := make([]string, 0, len(seen))
	for _, field := range []string{"server_url", "site_content_url", "api_version", "pat_name_env", "pat_secret_env", "default_workspace"} {
		if seen[field] {
			result = append(result, field)
			delete(seen, field)
		}
	}
	unknown := make([]string, 0, len(seen))
	for field := range seen {
		unknown = append(unknown, field)
	}
	sort.Strings(unknown)
	return append(result, unknown...)
}

func usageError(summary string) error {
	return &errs.Error{ID: "env.profile.update.usage", Kind: errs.KindUsage, Operation: "env.profile.update", Summary: summary}
}
