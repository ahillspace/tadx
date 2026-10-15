package profile

import (
	"context"
	"net/url"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
)

type Adder interface {
	Add(context.Context, AddProfile) (AddProfile, error)
	PreviewAdd(context.Context, AddProfile) (AddProfile, error)
}

type AddAction struct{ adder Adder }

func NewAdd(adder Adder) *AddAction { return &AddAction{adder: adder} }

func (a *AddAction) Execute(ctx context.Context, input AddInput) (AddOutput, error) {
	if a == nil || a.adder == nil {
		return AddOutput{}, &errs.Error{ID: "env.profile.add.unconfigured", Kind: errs.KindRuntime, Operation: "env.profile.add", Summary: "Environment profile creation is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the environment profile store before retrying."}
	}
	if strings.TrimSpace(input.Alias) == "" {
		return AddOutput{}, addUsageError("environment alias is required")
	}
	if !validServerURL(input.ServerURL) {
		return AddOutput{}, addUsageError("server URL must be an absolute HTTPS URL without credentials, query, or fragment")
	}
	if summary := variableReferenceProblem(input.PATNameEnv, input.PATSecretEnv); summary != "" {
		return AddOutput{}, variableReferenceError("env.profile.add.usage", "env.profile.add", summary)
	}
	if input.PATNameEnv != "" && strings.EqualFold(input.PATNameEnv, input.PATSecretEnv) {
		return AddOutput{}, addUsageError("PAT name and secret must use different environment variables")
	}
	if input.CacheMaxConcurrency < 0 || input.CacheMaxConcurrency > 256 {
		return AddOutput{}, addUsageError("cache maximum concurrency must be between 1 and 256, or omitted for the default")
	}
	add := a.adder.Add
	if input.Preview {
		add = a.adder.PreviewAdd
	}
	profile, err := add(ctx, AddProfile{Alias: input.Alias, ServerURL: input.ServerURL, SiteContentURL: input.SiteContentURL, AuthType: "pat", PATNameEnv: input.PATNameEnv, PATSecretEnv: input.PATSecretEnv, DefaultWorkspace: input.DefaultWorkspace, CacheMaxConcurrency: input.CacheMaxConcurrency})
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the new environment profile and alias, then retry.")
		id, summary := "env.profile.add.write", "Environment profile could not be added."
		if input.Preview {
			id, summary = "env.profile.add.preview", "Environment profile preview failed."
		}
		return AddOutput{}, &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "env.profile.add", Environment: input.Alias, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	var warnings []string
	if profile.MultipleEnvironments {
		warnings = []string{"Multiple environments are now configured. Remote writes require --env <name>. Reads still use your configured default."}
	}
	if input.Preview {
		return AddOutput{Status: "preview", Profile: profile, Help: []string{"Execution adds this profile after rechecking the configuration. The configuration has not been saved."}}, nil
	}
	return AddOutput{Warnings: warnings, Status: "added", Profile: profile, Help: []string{commandhint.Environment(profile.Alias, "auth", "status")}}, nil
}

func validServerURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && strings.EqualFold(parsed.Scheme, "https") && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

// variableReferenceProblem rejects PAT references that are not variable names
// before any write. The summary never contains the rejected value, because such
// a value is most likely a pasted PAT name or secret.
func variableReferenceProblem(name, secret string) string {
	for _, reference := range []struct{ label, value string }{
		{label: "PAT name", value: name},
		{label: "PAT secret", value: secret},
	} {
		if reference.value != "" && !config.ValidVariableReference(reference.value) {
			return reference.label + " environment-variable reference " + config.VariableReferenceRule + "; the value is not shown because it may be a secret"
		}
	}
	return ""
}

func variableReferenceError(id, operation, summary string) error {
	return &errs.Error{ID: id, Kind: errs.KindUsage, Operation: operation, Summary: summary, CorrectiveAction: "Pass the name of the environment variable that holds the value, never the PAT name or secret itself. Nothing was saved."}
}

func addUsageError(summary string) error {
	return &errs.Error{ID: "env.profile.add.usage", Kind: errs.KindUsage, Operation: "env.profile.add", Summary: summary}
}

type AddInput struct {
	Preview             bool   `json:"preview,omitempty"`
	Alias               string `json:"alias"`
	ServerURL           string `json:"server_url"`
	SiteContentURL      string `json:"site_content_url,omitempty"`
	PATNameEnv          string `json:"pat_name_env,omitempty"`
	PATSecretEnv        string `json:"pat_secret_env,omitempty"`
	DefaultWorkspace    string `json:"default_workspace,omitempty"`
	CacheMaxConcurrency int    `json:"cache_max_concurrency,omitempty"`
}

type AddProfile struct {
	MultipleEnvironments bool   `json:"-"`
	Alias                string `json:"alias"`
	ServerURL            string `json:"server_url"`
	SiteContentURL       string `json:"site_content_url,omitempty"`
	AuthType             string `json:"auth_type"`
	PATNameEnv           string `json:"pat_name_env"`
	PATSecretEnv         string `json:"pat_secret_env"`
	DefaultWorkspace     string `json:"default_workspace,omitempty"`
	CacheMaxConcurrency  int    `json:"cache_max_concurrency,omitempty"`
}

type AddOutput struct {
	Warnings []string   `json:"warnings,omitempty"`
	Status   string     `json:"status"`
	Profile  AddProfile `json:"environment"`
	Help     []string   `json:"help"`
}

type AddCompactProfile struct {
	Alias          string `json:"alias"`
	ServerURL      string `json:"server_url"`
	SiteContentURL string `json:"site_content_url,omitempty"`
}
type AddCompactResult struct {
	Warnings []string          `json:"warnings,omitempty"`
	Status   string            `json:"status"`
	Profile  AddCompactProfile `json:"environment"`
	Details  string            `json:"details"`
	Help     []string          `json:"help"`
}

func (o AddOutput) CompactOutput() any {
	return AddCompactResult{Warnings: o.Warnings, Status: o.Status, Profile: AddCompactProfile{Alias: o.Profile.Alias, ServerURL: o.Profile.ServerURL, SiteContentURL: o.Profile.SiteContentURL}, Details: "--full", Help: o.Help}
}
func (o AddOutput) FullOutput() any { return o }
