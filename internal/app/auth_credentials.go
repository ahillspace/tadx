package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	authops "github.com/ahillspace/tadx/actions/auth"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
)

type authCredentialResolver struct{ runtime *runtimeDependencies }

func (r authCredentialResolver) Resolve(_ context.Context, alias string) (authops.LoginTarget, error) {
	_, environment, err := r.runtime.environment(alias, true)
	if err != nil {
		return authops.LoginTarget{}, err
	}
	if strings.TrimSpace(os.Getenv(environment.Auth.PATNameEnv)) != "" || strings.TrimSpace(os.Getenv(environment.Auth.PATSecretEnv)) != "" {
		return authops.LoginTarget{}, &errs.Error{ID: "auth.login.environment_override", Kind: errs.KindOperation, Operation: "auth.login", Environment: environment.Alias, Summary: "Configured PAT environment variables override stored-credential login.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Retryable: errs.Bool(false), CorrectiveAction: fmt.Sprintf("Clear %s and %s from the calling process before stored-credential login. No PAT was requested, validated, or saved.", environment.Auth.PATNameEnv, environment.Auth.PATSecretEnv)}
	}
	return authops.LoginTarget{
		Environment:    environment.Alias,
		ServerURL:      environment.URL,
		SiteContentURL: environment.SiteContentURL,
	}, nil
}

type authLogoutResolver struct{ runtime *runtimeDependencies }

func (r authLogoutResolver) Resolve(_ context.Context, alias string, explicit bool) (authops.LogoutTarget, error) {
	// Resolve the credential reference from the current persisted configuration.
	configuration, err := config.Load(r.runtime.configPath)
	if err != nil {
		return authops.LogoutTarget{}, err
	}
	if alias == "" && !explicit {
		aliases := configuration.EnvironmentAliases()
		if len(aliases) != 1 {
			_, err := configuration.ResolveWriteEnvironment(alias)
			return authops.LogoutTarget{}, err
		}
		alias = aliases[0]
	}
	environment, err := configuration.EnvironmentForRepair(alias)
	if err != nil {
		return authops.LogoutTarget{}, err
	}
	reference := environment.Auth.CredentialRef
	invalidReference := false
	if invalid, ok := configuration.InvalidEnvironments[alias]; ok {
		reference = invalid.CredentialRef
		for _, violation := range invalid.Violations {
			if strings.HasSuffix(violation.Field, "credential_ref") || violation.Field == "auth" {
				invalidReference = reference == ""
			}
		}
	}
	return authops.LogoutTarget{Environment: environment.Alias, StoredCredentialReferencePresent: reference != "", StoredCredentialReferenceInvalid: invalidReference || reference == "" && environment.Auth.CredentialRef != "", EnvironmentCredentialsAvailable: config.ValidVariableReference(environment.Auth.PATNameEnv) && config.ValidVariableReference(environment.Auth.PATSecretEnv) && strings.TrimSpace(os.Getenv(environment.Auth.PATNameEnv)) != "" && strings.TrimSpace(os.Getenv(environment.Auth.PATSecretEnv)) != ""}, nil
}

type loginAuthenticator struct{ runtime *runtimeDependencies }

func (a loginAuthenticator) Authenticate(ctx context.Context, target authops.LoginTarget, credential authops.LoginCredential) (authops.LoginAuthentication, error) {
	session, err := a.runtime.commandSessions().AuthenticateCredentials(ctx, coreauth.Target{
		Environment:    target.Environment,
		ServerURL:      target.ServerURL,
		SiteContentURL: target.SiteContentURL,
		Operation:      "auth.login",
	}, coreauth.PATCredentials{Name: credential.PATName, Secret: credential.PATSecret}, commandSigner{runtime: a.runtime})
	if err != nil {
		return authops.LoginAuthentication{}, err
	}
	return authops.LoginAuthentication{SiteLUID: session.SiteLUID(), UserLUID: session.UserLUID()}, nil
}

type authCredentialStore struct{ runtime *runtimeDependencies }

func (s authCredentialStore) Store(ctx context.Context, target authops.LoginTarget, credential authops.LoginCredential) (authops.LoginStoreResult, error) {
	if s.runtime == nil || s.runtime.patStore == nil {
		return authops.LoginStoreResult{}, errors.New("native credential storage is not configured")
	}
	var environmentVariablesOverride bool
	_, err := config.UpdateWithRollback(s.runtime.configPath, false, func(configuration config.Config) (config.Config, func() error, error) {
		environment, resolveErr := configuration.EnvironmentForRepair(target.Environment)
		if resolveErr != nil {
			return config.Config{}, nil, resolveErr
		}
		if environment.URL != target.ServerURL || environment.SiteContentURL != target.SiteContentURL {
			return config.Config{}, nil, errors.New("environment target changed after PAT validation")
		}
		resolved, resolveErr := configuration.ResolveEnvironment(target.Environment)
		if resolveErr != nil {
			return config.Config{}, nil, resolveErr
		}
		name, namePresent := os.LookupEnv(resolved.Auth.PATNameEnv)
		secret, secretPresent := os.LookupEnv(resolved.Auth.PATSecretEnv)
		environmentVariablesOverride = namePresent && strings.TrimSpace(name) != "" && secretPresent && strings.TrimSpace(secret) != ""
		if environment.Auth.CredentialRef != "" {
			reference := coreauth.CredentialReference(environment.Auth.CredentialRef)
			replaceErr := s.runtime.patStore.ReplacePAT(ctx, reference, coreauth.CredentialTarget{ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL}, credential.PATName, credential.PATSecret)
			if replaceErr != nil {
				return config.Config{}, nil, replaceErr
			}
			return configuration, nil, config.ErrNoChange
		}
		stored, storeErr := s.runtime.patStore.StorePAT(ctx, coreauth.CredentialTarget{ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL}, credential.PATName, credential.PATSecret)
		if storeErr != nil {
			return config.Config{}, nil, storeErr
		}
		environment.Auth.CredentialRef = string(stored)
		configuration.Environments[target.Environment] = environment
		return configuration, func() error { return s.runtime.patStore.DeletePAT(context.WithoutCancel(ctx), stored) }, nil
	})
	if err != nil {
		return authops.LoginStoreResult{}, err
	}
	return authops.LoginStoreResult{EnvironmentVariablesOverride: environmentVariablesOverride}, nil
}

func (s authCredentialStore) Remove(ctx context.Context, target authops.LogoutTarget) (authops.LogoutRemoveResult, error) {
	if s.runtime == nil || s.runtime.patStore == nil {
		return authops.LogoutRemoveResult{}, errors.New("native credential storage is not configured")
	}
	removed := false
	var reference coreauth.CredentialReference
	_, err := config.UpdateWithPostSave(s.runtime.configPath, false, func(configuration config.Config) (config.Config, func() error, error) {
		environment, resolveErr := configuration.EnvironmentForRepair(target.Environment)
		if resolveErr != nil {
			return config.Config{}, nil, resolveErr
		}
		referenceValue := environment.Auth.CredentialRef
		if invalid, ok := configuration.InvalidEnvironments[target.Environment]; ok {
			referenceValue = invalid.CredentialRef
		}
		if referenceValue == "" {
			return config.Config{}, nil, config.ErrNoChange
		}
		reference = coreauth.CredentialReference(referenceValue)
		if err := configuration.ClearCredentialReference(target.Environment); err != nil {
			return config.Config{}, nil, err
		}
		removed = true
		return configuration, func() error {
			deleteErr := s.runtime.patStore.DeletePAT(ctx, reference)
			if credentialNotFound(deleteErr) {
				return nil
			}
			return deleteErr
		}, nil
	})
	if err != nil {
		return authops.LogoutRemoveResult{}, orphanedCredentialError(err, reference)
	}
	return authops.LogoutRemoveResult{Removed: removed}, nil
}

// orphanedCredentialError names the stored entry that remains when the cleared
// reference took effect but its deletion failed, because logout no longer finds it.
func orphanedCredentialError(err error, reference coreauth.CredentialReference) error {
	var installed *config.InstalledError
	if !errors.As(err, &installed) || installed.ExternalCommitConfirmed() {
		return err
	}
	return &errs.Error{Kind: errs.KindOperation, Summary: "The stored PAT could not be deleted after its reference was cleared.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Remove the OS credential store entry " + coreauth.CredentialStoreEntry(reference) + " by hand; logout no longer references it."}
}

func credentialNotFound(err error) bool {
	var storeError *coreauth.CredentialStoreError
	return errors.As(err, &storeError) && storeError.Kind == coreauth.CredentialStoreNotFound
}
