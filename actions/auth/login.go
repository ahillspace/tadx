package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

// LoginResolver resolves one configured environment without reading credentials.
type LoginResolver interface {
	Resolve(context.Context, string) (LoginTarget, error)
}

// LoginAuthenticator validates an in-memory PAT against Tableau.
type LoginAuthenticator interface {
	Authenticate(context.Context, LoginTarget, LoginCredential) (LoginAuthentication, error)
}

// LoginStore persists a validated PAT in TADX's native OS credential store.
type LoginStore interface {
	Store(context.Context, LoginTarget, LoginCredential) (LoginStoreResult, error)
}

// LoginPreflight resolves the environment before CLI plumbing requests any PAT input.
func (a *Service) LoginPreflight(ctx context.Context, environment string) error {
	if a == nil || a.LoginResolver == nil || a.StatusLookup == nil {
		return usage("Authentication setup is unavailable.")
	}
	target, err := a.LoginResolver.Resolve(ctx, environment)
	if err != nil {
		return err
	}
	return a.rejectLoginOverride(target)
}

// Login validates the PAT before allowing storage.
func (a *Service) Login(ctx context.Context, input LoginInput) (LoginOutput, error) {
	if a == nil || a.LoginResolver == nil || a.LoginAuthenticator == nil || a.LoginStore == nil || a.StatusLookup == nil {
		return LoginOutput{}, &errs.Error{ID: "auth.login.unconfigured", Kind: errs.KindRuntime, Operation: "auth.login", Summary: "PAT login is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure PAT validation and OS credential storage before retrying."}
	}
	if strings.TrimSpace(input.PATName) == "" {
		return LoginOutput{}, usage("PAT name is required")
	}
	if strings.TrimSpace(input.PATSecret) == "" {
		return LoginOutput{}, usage("PAT secret is required")
	}

	target, err := a.LoginResolver.Resolve(ctx, input.Environment)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias, then retry.")
		return LoginOutput{}, &errs.Error{ID: "auth.login.resolve", Kind: errs.KindOperation, Operation: "auth.login", Environment: input.Environment, Summary: "Environment resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	if err := a.rejectLoginOverride(target); err != nil {
		return LoginOutput{}, err
	}
	credential := LoginCredential{PATName: input.PATName, PATSecret: input.PATSecret}
	identity, err := a.LoginAuthenticator.Authenticate(ctx, target, credential)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Verify the PAT credentials and Tableau target. No credential was saved.")
		return LoginOutput{}, &errs.Error{ID: "auth.login.authenticate", Kind: errs.KindOperation, Operation: "auth.login", Environment: target.Environment, Site: target.SiteContentURL, Summary: "PAT validation failed. No credential was saved.", Cause: err, Retryable: retryable, CorrectiveAction: ensureNoSaveAdvice(advice), TableauRequestID: errs.TableauRequestID(err)}
	}
	if strings.TrimSpace(identity.SiteLUID) == "" || strings.TrimSpace(identity.UserLUID) == "" {
		return LoginOutput{}, &errs.Error{ID: "auth.login.authenticate", Kind: errs.KindOperation, Operation: "auth.login", Environment: target.Environment, Site: target.SiteContentURL, Summary: "PAT validation returned incomplete identity. No credential was saved.", Retryable: errs.Bool(false), CorrectiveAction: "Review the Tableau sign-in response. No credential was saved."}
	}

	stored, err := a.LoginStore.Store(ctx, target, credential)
	if configurationInstalled(err) {
		retryable, advice := errs.CompleteRetryAdvice(err, "The PAT was saved. Repair access to the configuration directory, then confirm the stored credential.")
		return LoginOutput{}, &errs.Error{ID: "auth.login.store", Kind: errs.KindOperation, Operation: "auth.login", Environment: target.Environment, Site: target.SiteContentURL, Summary: "The PAT was saved, but the configuration could not be made durable.", Cause: err, Retryable: retryable, CorrectiveAction: advice, Phase: errs.PhasePersistence, Outcome: errs.OutcomeConfirmed}
	}
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Repair the OS credential store, then retry. The PAT was validated but not saved.")
		return LoginOutput{}, &errs.Error{ID: "auth.login.store", Kind: errs.KindOperation, Operation: "auth.login", Environment: target.Environment, Site: target.SiteContentURL, Summary: "The PAT was validated, but credential storage failed.", Cause: err, Retryable: retryable, CorrectiveAction: ensureNotSavedAdvice(advice)}
	}

	warnings := []string(nil)
	if stored.EnvironmentVariablesOverride {
		warnings = append(warnings, "The configured environment variables currently override this stored PAT.")
	}
	return LoginOutput{
		Status: "stored", Environment: target.Environment, CredentialSource: CredentialSourceOS, Validated: true,
		SiteLUID: identity.SiteLUID, UserLUID: identity.UserLUID, Warnings: warnings,
		Help: []string{},
	}, nil
}

func (a *Service) rejectLoginOverride(target LoginTarget) error {
	name, _ := a.StatusLookup.LookupEnv(target.PATNameVariable)
	secret, _ := a.StatusLookup.LookupEnv(target.PATSecretVariable)
	if strings.TrimSpace(name) == "" && strings.TrimSpace(secret) == "" {
		return nil
	}
	return &errs.Error{ID: "auth.login.environment_override", Kind: errs.KindOperation, Operation: "auth.login", Environment: target.Environment, Summary: "Configured PAT environment variables override stored-credential login.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Retryable: errs.Bool(false), CorrectiveAction: fmt.Sprintf("Clear %s and %s from the calling process before stored-credential login. No PAT was requested, validated, or saved.", target.PATNameVariable, target.PATSecretVariable)}
}

// configurationInstalled reports a store failure after the configuration change
// took effect, so the credential is saved even though the update failed.
func configurationInstalled(err error) bool {
	var installed interface{ ConfigurationInstalled() bool }
	return errors.As(err, &installed) && installed.ConfigurationInstalled()
}

func usage(summary string) error {
	return &errs.Error{ID: "auth.login.usage", Kind: errs.KindUsage, Operation: "auth.login", Summary: summary, Retryable: errs.Bool(false), CorrectiveAction: "Provide an explicit environment and enter a complete PAT through an interactive terminal."}
}

func ensureNoSaveAdvice(value string) string {
	if strings.Contains(value, "No credential was saved") {
		return value
	}
	return strings.TrimSpace(value) + " No credential was saved."
}

func ensureNotSavedAdvice(value string) string {
	if strings.Contains(value, "not saved") {
		return value
	}
	return strings.TrimSpace(value) + " The PAT was not saved."
}

const CredentialSourceOS = "os_credential_store"

// Input contains one explicit environment and the credentials read by CLI plumbing.
type LoginInput struct {
	Environment string
	PATName     string
	PATSecret   string
}

func (i LoginInput) String() string {
	return fmt.Sprintf("PAT login for environment %q ([REDACTED])", i.Environment)
}
func (i LoginInput) GoString() string { return i.String() }
func (i LoginInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Environment string `json:"environment"`
	}{Environment: i.Environment})
}

// Target contains nonsecret authentication and storage context.
type LoginTarget struct {
	Environment       string
	ServerURL         string
	SiteContentURL    string
	APIVersion        string
	PATNameVariable   string
	PATSecretVariable string
}

// Credential carries PAT values only between bounded authentication dependencies.
type LoginCredential struct {
	PATName   string
	PATSecret string
}

func (LoginCredential) String() string     { return "PAT credential ([REDACTED])" }
func (c LoginCredential) GoString() string { return c.String() }
func (LoginCredential) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Redacted bool `json:"redacted"`
	}{Redacted: true})
}

// Authentication is the authoritative identity returned by Tableau validation.
type LoginAuthentication struct {
	SiteLUID string
	UserLUID string
}

// StoreResult reports nonsecret state observed while saving the credential.
type LoginStoreResult struct {
	EnvironmentVariablesOverride bool
}

// Output reports credential persistence without exposing PAT values.
type LoginOutput struct {
	Status           string   `json:"status"`
	Environment      string   `json:"environment"`
	CredentialSource string   `json:"credential_source"`
	Validated        bool     `json:"validated"`
	SiteLUID         string   `json:"site_luid,omitempty"`
	UserLUID         string   `json:"user_luid,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
	Help             []string `json:"help"`
}

// CompactResult omits identity details available through --full.
type LoginCompactResult struct {
	Status           string   `json:"status"`
	Environment      string   `json:"environment"`
	CredentialSource string   `json:"credential_source"`
	Validated        bool     `json:"validated"`
	Warnings         []string `json:"warnings,omitempty"`
	SiteLUID         string   `json:"site_luid,omitempty"`
	UserLUID         string   `json:"user_luid,omitempty"`
	Details          string   `json:"details"`
	Help             []string `json:"help"`
}

// CompactOutput returns the bounded default result.
func (o LoginOutput) CompactOutput() any {
	return LoginCompactResult{
		Status: o.Status, Environment: o.Environment, CredentialSource: o.CredentialSource,
		Validated: o.Validated, Warnings: o.Warnings, SiteLUID: o.SiteLUID, UserLUID: o.UserLUID, Details: "--full", Help: o.Help,
	}
}

// FullOutput returns the validated Tableau identity without credential values.
func (o LoginOutput) FullOutput() any { return o }
