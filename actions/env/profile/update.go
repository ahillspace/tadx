package profile

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type Updater interface {
	Update(context.Context, string, Patch) (UpdateResult, error)
}

type UpdateAction struct{ updater Updater }

func NewUpdate(updater Updater) *UpdateAction { return &UpdateAction{updater: updater} }

func (a *UpdateAction) Execute(ctx context.Context, input UpdateInput) (UpdateOutput, error) {
	if a == nil || a.updater == nil {
		return UpdateOutput{}, &errs.Error{ID: "env.profile.update.unconfigured", Kind: errs.KindRuntime, Operation: "env.profile.update", Summary: "Environment profile update is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the environment profile store before retrying."}
	}
	if strings.TrimSpace(input.Alias) == "" {
		return UpdateOutput{}, updateUsageError("environment alias is required")
	}
	if !input.Patch.Any() {
		return UpdateOutput{}, updateUsageError("at least one profile field must be supplied")
	}
	if value := input.Patch.CacheMaxConcurrency; value.Set && (value.Value < 0 || value.Value > 256) {
		return UpdateOutput{}, updateUsageError("cache maximum concurrency must be between 1 and 256, or cleared for the default")
	}
	if input.Patch.ServerURL.Set && !validServerURL(input.Patch.ServerURL.Value) {
		return UpdateOutput{}, updateUsageError("server URL must be an absolute HTTPS URL without credentials, query, or fragment")
	}
	if input.Patch.PATNameEnv.Set && input.Patch.PATSecretEnv.Set && input.Patch.PATNameEnv.Value != "" && strings.EqualFold(input.Patch.PATNameEnv.Value, input.Patch.PATSecretEnv.Value) {
		return UpdateOutput{}, updateUsageError("PAT name and secret must use different environment variables")
	}
	update := a.updater.Update
	if input.Preview {
		previewer, ok := a.updater.(interface {
			PreviewUpdate(context.Context, string, Patch) (UpdateResult, error)
		})
		if !ok {
			return UpdateOutput{}, &errs.Error{ID: "env.profile.update.preview", Kind: errs.KindRuntime, Operation: "env.profile.update", Summary: "Profile preview is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure a read-only profile preview store."}
		}
		update = previewer.PreviewUpdate
	}
	result, err := update(ctx, input.Alias, input.Patch)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias and requested fields, then retry.")
		id, summary := "env.profile.update.write", "Environment profile could not be updated."
		if input.Preview {
			id, summary = "env.profile.update.preview", "Environment profile preview failed."
		}
		return UpdateOutput{}, &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "env.profile.update", Environment: input.Alias, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	changedFields := normalizeChangedFields(result.ChangedFields)
	if input.Preview {
		return UpdateOutput{Status: "preview", Profile: result.Profile, ChangedFields: changedFields, Help: []string{"Execution applies these changed fields after rechecking the configuration. The configuration has not been saved."}}, nil
	}
	status := "unchanged"
	if len(changedFields) > 0 {
		status = "updated"
	}
	return UpdateOutput{Status: status, Profile: result.Profile, ChangedFields: changedFields, Help: []string{commandhint.Command("env", "get", result.Profile.Alias)}}, nil
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
	slices.Sort(unknown)
	return append(result, unknown...)
}

func updateUsageError(summary string) error {
	return &errs.Error{ID: "env.profile.update.usage", Kind: errs.KindUsage, Operation: "env.profile.update", Summary: summary}
}

type StringField struct {
	Set   bool   `json:"set"`
	Value string `json:"value"`
}

type IntField struct {
	Set   bool `json:"set"`
	Value int  `json:"value"`
}

type Patch struct {
	ServerURL           StringField `json:"server_url"`
	SiteContentURL      StringField `json:"site_content_url"`
	APIVersion          StringField `json:"api_version"`
	PATNameEnv          StringField `json:"pat_name_env"`
	PATSecretEnv        StringField `json:"pat_secret_env"`
	DefaultWorkspace    StringField `json:"default_workspace"`
	CacheMaxConcurrency IntField    `json:"cache_max_concurrency"`
}

func (p Patch) Any() bool {
	return p.ServerURL.Set || p.SiteContentURL.Set || p.APIVersion.Set || p.PATNameEnv.Set || p.PATSecretEnv.Set || p.DefaultWorkspace.Set || p.CacheMaxConcurrency.Set
}

type UpdateInput struct {
	Preview bool   `json:"preview,omitempty"`
	Alias   string `json:"alias"`
	Patch   Patch  `json:"patch"`
}

type UpdateProfile struct {
	Alias               string `json:"alias"`
	Default             bool   `json:"default"`
	ServerURL           string `json:"server_url"`
	SiteContentURL      string `json:"site_content_url"`
	APIVersion          string `json:"api_version"`
	AuthType            string `json:"auth_type"`
	PATNameEnv          string `json:"pat_name_env"`
	PATSecretEnv        string `json:"pat_secret_env"`
	DefaultWorkspace    string `json:"default_workspace"`
	CacheMaxConcurrency int    `json:"cache_max_concurrency"`
}

type UpdateResult struct {
	Profile       UpdateProfile
	ChangedFields []string
}

type UpdateOutput struct {
	Status        string        `json:"status"`
	Profile       UpdateProfile `json:"environment"`
	ChangedFields []string      `json:"changed_fields,omitempty"`
	Help          []string      `json:"help"`
}

type UpdateCompactResult struct {
	Status        string         `json:"status"`
	Environment   map[string]any `json:"environment"`
	ChangedFields []string       `json:"changed_fields,omitempty"`
	Details       string         `json:"details"`
	Help          []string       `json:"help"`
}
type UpdateFullResult struct {
	Status        string        `json:"status"`
	Profile       UpdateProfile `json:"environment"`
	ChangedFields []string      `json:"changed_fields,omitempty"`
	Help          []string      `json:"help"`
}

func (o UpdateOutput) CompactOutput() any {
	profile := map[string]any{"alias": o.Profile.Alias, "server_url": o.Profile.ServerURL, "site_content_url": o.Profile.SiteContentURL}
	values := map[string]any{
		"api_version":           o.Profile.APIVersion,
		"pat_name_env":          o.Profile.PATNameEnv,
		"pat_secret_env":        o.Profile.PATSecretEnv,
		"default_workspace":     o.Profile.DefaultWorkspace,
		"cache_max_concurrency": o.Profile.CacheMaxConcurrency,
	}
	for _, field := range o.ChangedFields {
		if value, ok := values[field]; ok {
			profile[field] = value
		}
	}
	return UpdateCompactResult{Status: o.Status, Environment: profile, ChangedFields: o.ChangedFields, Details: "--full", Help: o.Help}
}
func (o UpdateOutput) FullOutput() any {
	return UpdateFullResult{Status: o.Status, Profile: o.Profile, ChangedFields: o.ChangedFields, Help: o.Help}
}
