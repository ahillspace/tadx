package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	authlogin "github.com/ahillspace/tadx/actions/auth/login"
	authlogout "github.com/ahillspace/tadx/actions/auth/logout"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/tableau"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
)

type authCredentialResolver struct{ runtime *runtimeDependencies }

func (r authCredentialResolver) Resolve(_ context.Context, alias string) (authlogin.Target, error) {
	_, environment, err := r.runtime.environment(alias, true)
	if err != nil {
		return authlogin.Target{}, err
	}
	return authlogin.Target{
		Environment:    environment.Alias,
		ServerURL:      environment.URL,
		SiteContentURL: environment.SiteContentURL,
		APIVersion:     environment.APIVersion,
	}, nil
}

type authLogoutResolver struct{ runtime *runtimeDependencies }

func (r authLogoutResolver) Resolve(_ context.Context, alias string) (authlogout.Target, error) {
	_, environment, err := r.runtime.environment(alias, true)
	if err != nil {
		return authlogout.Target{}, err
	}
	return authlogout.Target{Environment: environment.Alias}, nil
}

type loginAuthenticator struct{ runtime *runtimeDependencies }

func (a loginAuthenticator) Authenticate(ctx context.Context, target authlogin.Target, credential authlogin.Credential) (authlogin.Authentication, error) {
	transport := tableau.NewTransport(a.runtime.httpClient, target.APIVersion, func() string { return a.runtime.correlationID })
	lookup := coreauth.LookupEnvFunc(func(name string) (string, bool) {
		switch name {
		case "TADX_INTERACTIVE_PAT_NAME":
			return credential.PATName, true
		case "TADX_INTERACTIVE_PAT_SECRET":
			return credential.PATSecret, true
		default:
			return "", false
		}
	})
	provider := coreauth.NewPATProvider(lookup, tableauauth.NewClient(transport))
	session, err := provider.Authenticate(ctx, coreauth.Target{
		Environment:       target.Environment,
		ServerURL:         target.ServerURL,
		SiteContentURL:    target.SiteContentURL,
		PATNameVariable:   "TADX_INTERACTIVE_PAT_NAME",
		PATSecretVariable: "TADX_INTERACTIVE_PAT_SECRET",
		Operation:         "auth.login",
	})
	if err != nil {
		return authlogin.Authentication{}, err
	}
	return authlogin.Authentication{SiteLUID: session.SiteLUID(), UserLUID: session.UserLUID()}, nil
}

type authCredentialStore struct{ runtime *runtimeDependencies }

func (s authCredentialStore) Store(ctx context.Context, target authlogin.Target, credential authlogin.Credential) (authlogin.StoreResult, error) {
	if s.runtime == nil || s.runtime.patStore == nil {
		return authlogin.StoreResult{}, errors.New("native credential storage is not configured")
	}
	var environmentVariablesOverride bool
	_, err := config.UpdateWithRollback(s.runtime.configPath, false, func(configuration config.Config) (config.Config, func() error, error) {
		environment, exists := configuration.Environments[target.Environment]
		if !exists {
			return config.Config{}, nil, fmt.Errorf("environment %q does not exist", target.Environment)
		}
		if environment.URL != target.ServerURL || environment.SiteContentURL != target.SiteContentURL {
			return config.Config{}, nil, errors.New("environment target changed after PAT validation")
		}
		name, namePresent := os.LookupEnv(effectivePATNameVariable(target.Environment, environment.Auth.PATNameEnv))
		secret, secretPresent := os.LookupEnv(effectivePATSecretVariable(target.Environment, environment.Auth.PATSecretEnv))
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
		return authlogin.StoreResult{}, err
	}
	return authlogin.StoreResult{EnvironmentVariablesOverride: environmentVariablesOverride}, nil
}

func (s authCredentialStore) Remove(ctx context.Context, target authlogout.Target) (authlogout.RemoveResult, error) {
	if s.runtime == nil || s.runtime.patStore == nil {
		return authlogout.RemoveResult{}, errors.New("native credential storage is not configured")
	}
	removed := false
	var reference coreauth.CredentialReference
	_, err := config.UpdateWithPostSave(s.runtime.configPath, false, func(configuration config.Config) (config.Config, func() error, error) {
		environment, exists := configuration.Environments[target.Environment]
		if !exists {
			return config.Config{}, nil, fmt.Errorf("environment %q does not exist", target.Environment)
		}
		if environment.Auth.CredentialRef == "" {
			return config.Config{}, nil, config.ErrNoChange
		}
		reference = coreauth.CredentialReference(environment.Auth.CredentialRef)
		environment.Auth.CredentialRef = ""
		configuration.Environments[target.Environment] = environment
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
		return authlogout.RemoveResult{}, err
	}
	return authlogout.RemoveResult{Removed: removed}, nil
}

func credentialNotFound(err error) bool {
	var storeError *coreauth.CredentialStoreError
	return errors.As(err, &storeError) && storeError.Kind == coreauth.CredentialStoreNotFound
}

func effectivePATNameVariable(alias, configured string) string {
	if configured != "" {
		return configured
	}
	name, _ := config.DefaultPATVariableNames(alias)
	return name
}

func effectivePATSecretVariable(alias, configured string) string {
	if configured != "" {
		return configured
	}
	_, secret := config.DefaultPATVariableNames(alias)
	return secret
}
