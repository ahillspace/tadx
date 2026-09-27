// Package status implements auth.status.
package status

import (
	"context"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type Resolver interface {
	Resolve(context.Context, string) (Target, error)
}
type LookupEnv interface{ LookupEnv(string) (string, bool) }

type Action struct {
	resolver Resolver
	lookup   LookupEnv
}

func New(resolver Resolver, lookup LookupEnv) *Action {
	return &Action{resolver: resolver, lookup: lookup}
}

func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil || a.lookup == nil {
		return Output{}, &errs.Error{ID: "auth.status.unconfigured", Kind: errs.KindRuntime, Operation: "auth.status", Summary: "Authentication status is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure environment and process-variable resolution before retrying."}
	}
	target, err := a.resolver.Resolve(ctx, input.Environment)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the selected environment configuration, then retry.")
		return Output{}, &errs.Error{ID: "auth.status.resolve", Kind: errs.KindOperation, Operation: "auth.status", Environment: input.Environment, Summary: "Authentication configuration could not be resolved.", Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	return Inspect(target, a.lookup), nil
}

// Inspect reports local readiness for an already resolved configuration target.
// It reads process variables at the time of inspection and never opens the store.
func Inspect(target Target, lookup LookupEnv) Output {
	name, nameExists := lookup.LookupEnv(target.PATNameVariable)
	secret, secretExists := lookup.LookupEnv(target.PATSecretVariable)
	namePresent := nameExists && strings.TrimSpace(name) != ""
	secretPresent := secretExists && strings.TrimSpace(secret) != ""
	state := "incomplete"
	source := "none"
	if namePresent && secretPresent {
		state = "ready"
		source = "environment"
	} else if namePresent || secretPresent {
		source = "environment"
	} else if target.StoredCredentialReferencePresent {
		state = "ready"
		source = "os_credential_store"
	}
	var missing []string
	help := []string{"Optional live verification: " + commandhint.Environment(target.Environment, "auth", "check")}
	if state == "incomplete" {
		if !namePresent {
			missing = append(missing, target.PATNameVariable)
		}
		if !secretPresent {
			missing = append(missing, target.PATSecretVariable)
		}
		help = []string{"Set the missing configured PAT variables. Variable-reference flags take names, not secret values."}
		if !namePresent && !secretPresent {
			help = append(help, commandhint.Environment(target.Environment, "auth", "login"))
		}
	}
	return Output{Status: state, Verification: "local_readiness_only", MissingVariables: missing, Environment: target.Environment, Default: target.Default, ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL, APIVersion: target.APIVersion, AuthType: target.AuthType, PATNameVariable: target.PATNameVariable, PATSecretVariable: target.PATSecretVariable, PATNamePresent: namePresent, PATSecretPresent: secretPresent, StoredCredentialReferencePresent: target.StoredCredentialReferencePresent, CredentialSource: source, DefaultWorkspace: target.DefaultWorkspace, Help: help}
}

type Input struct {
	Environment string `json:"environment,omitempty"`
}
type Target struct {
	Environment                      string
	Default                          bool
	ServerURL                        string
	SiteContentURL                   string
	APIVersion                       string
	AuthType                         string
	PATNameVariable                  string
	PATSecretVariable                string
	StoredCredentialReferencePresent bool
	DefaultWorkspace                 string
}

type Output struct {
	Verification                     string   `json:"verification"`
	MissingVariables                 []string `json:"missing_variables,omitempty"`
	Status                           string   `json:"status"`
	Environment                      string   `json:"environment"`
	Default                          bool     `json:"default"`
	ServerURL                        string   `json:"server_url"`
	SiteContentURL                   string   `json:"site_content_url,omitempty"`
	APIVersion                       string   `json:"api_version,omitempty"`
	AuthType                         string   `json:"auth_type"`
	PATNameVariable                  string   `json:"pat_name_env"`
	PATSecretVariable                string   `json:"pat_secret_env"`
	PATNamePresent                   bool     `json:"pat_name_present"`
	PATSecretPresent                 bool     `json:"pat_secret_present"`
	StoredCredentialReferencePresent bool     `json:"stored_credential_reference_present"`
	CredentialSource                 string   `json:"credential_source"`
	DefaultWorkspace                 string   `json:"default_workspace,omitempty"`
	Help                             []string `json:"help"`
}

type CompactResult struct {
	Verification                     string   `json:"verification"`
	MissingVariables                 []string `json:"missing_variables,omitempty"`
	Status                           string   `json:"status"`
	Environment                      string   `json:"environment"`
	ServerURL                        string   `json:"server_url"`
	SiteContentURL                   string   `json:"site_content_url,omitempty"`
	PATNamePresent                   bool     `json:"pat_name_present"`
	PATSecretPresent                 bool     `json:"pat_secret_present"`
	StoredCredentialReferencePresent bool     `json:"stored_credential_reference_present"`
	CredentialSource                 string   `json:"credential_source"`
	Details                          string   `json:"details"`
	Help                             []string `json:"help"`
}

func (o Output) CompactOutput() any {
	return CompactResult{Verification: o.Verification, MissingVariables: o.MissingVariables, Status: o.Status, Environment: o.Environment, ServerURL: o.ServerURL, SiteContentURL: o.SiteContentURL, PATNamePresent: o.PATNamePresent, PATSecretPresent: o.PATSecretPresent, StoredCredentialReferencePresent: o.StoredCredentialReferencePresent, CredentialSource: o.CredentialSource, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	return o
}
