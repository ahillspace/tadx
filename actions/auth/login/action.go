package login

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

// Resolver resolves one configured environment without reading credentials.
type Resolver interface {
	Resolve(context.Context, string) (Target, error)
}

// Authenticator validates an in-memory PAT against Tableau.
type Authenticator interface {
	Authenticate(context.Context, Target, Credential) (Authentication, error)
}

// Store persists a validated PAT in TADX's native OS credential store.
type Store interface {
	Store(context.Context, Target, Credential) (StoreResult, error)
}

// Action validates and stores one PAT credential.
type Action struct {
	resolver      Resolver
	authenticator Authenticator
	store         Store
}

// New creates auth.login.
func New(resolver Resolver, authenticator Authenticator, store Store) *Action {
	return &Action{resolver: resolver, authenticator: authenticator, store: store}
}

// Preflight resolves the environment before CLI plumbing requests any PAT input.
func (a *Action) Preflight(ctx context.Context, environment string) error {
	if a == nil || a.resolver == nil {
		return usage("Authentication setup is unavailable.")
	}
	_, err := a.resolver.Resolve(ctx, environment)
	return err
}

// Execute validates the PAT before allowing storage.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil || a.authenticator == nil || a.store == nil {
		return Output{}, &errs.Error{ID: "auth.login.unconfigured", Kind: errs.KindRuntime, Operation: "auth.login", Summary: "PAT login is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure PAT validation and OS credential storage before retrying."}
	}
	if strings.TrimSpace(input.Environment) == "" {
		return Output{}, usage("--environment is required")
	}
	if strings.TrimSpace(input.PATName) == "" {
		return Output{}, usage("PAT name is required")
	}
	if strings.TrimSpace(input.PATSecret) == "" {
		return Output{}, usage("PAT secret is required")
	}

	target, err := a.resolver.Resolve(ctx, input.Environment)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias, then retry.")
		return Output{}, &errs.Error{ID: "auth.login.resolve", Kind: errs.KindOperation, Operation: "auth.login", Environment: input.Environment, Summary: "Environment resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	if strings.TrimSpace(target.Environment) == "" || strings.TrimSpace(target.ServerURL) == "" {
		return Output{}, &errs.Error{ID: "auth.login.target", Kind: errs.KindOperation, Operation: "auth.login", Environment: input.Environment, Summary: "Selected environment is incomplete.", Cause: errors.New("environment and server URL are required"), Retryable: errs.Bool(false), CorrectiveAction: "Complete the selected environment profile before retrying."}
	}

	credential := Credential{PATName: input.PATName, PATSecret: input.PATSecret}
	identity, err := a.authenticator.Authenticate(ctx, target, credential)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Verify the PAT credentials and Tableau target. No credential was saved.")
		return Output{}, &errs.Error{ID: "auth.login.authenticate", Kind: errs.KindOperation, Operation: "auth.login", Environment: target.Environment, Site: target.SiteContentURL, Summary: "PAT validation failed. No credential was saved.", Cause: err, Retryable: retryable, CorrectiveAction: ensureNoSaveAdvice(advice), TableauRequestID: errs.TableauRequestID(err)}
	}
	if strings.TrimSpace(identity.SiteLUID) == "" || strings.TrimSpace(identity.UserLUID) == "" {
		return Output{}, &errs.Error{ID: "auth.login.authenticate", Kind: errs.KindOperation, Operation: "auth.login", Environment: target.Environment, Site: target.SiteContentURL, Summary: "PAT validation returned incomplete identity. No credential was saved.", Retryable: errs.Bool(false), CorrectiveAction: "Review the Tableau sign-in response. No credential was saved."}
	}

	stored, err := a.store.Store(ctx, target, credential)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Repair the OS credential store, then retry. The PAT was validated but not saved.")
		return Output{}, &errs.Error{ID: "auth.login.store", Kind: errs.KindOperation, Operation: "auth.login", Environment: target.Environment, Site: target.SiteContentURL, Summary: "The PAT was validated, but credential storage failed.", Cause: err, Retryable: retryable, CorrectiveAction: ensureNotSavedAdvice(advice)}
	}

	warnings := []string(nil)
	if stored.EnvironmentVariablesOverride {
		warnings = append(warnings, "The configured environment variables currently override this stored PAT.")
	}
	return Output{
		Status: "stored", Environment: target.Environment, CredentialSource: CredentialSourceOS, Validated: true,
		SiteLUID: identity.SiteLUID, UserLUID: identity.UserLUID, Warnings: warnings,
		Help: []string{},
	}, nil
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
