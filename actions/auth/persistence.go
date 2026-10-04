package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
)

// CredentialPersistence coordinates configuration and native credential effects.
// The configuration package retains locking and replacement mechanics.
type CredentialPersistence struct {
	configPath string
	patStore   coreauth.PATStore
	lookup     StatusLookupEnv
}

// NewCredentialPersistence binds the two stores without opening either one.
func NewCredentialPersistence(configPath string, patStore coreauth.PATStore, lookup StatusLookupEnv) *CredentialPersistence {
	return &CredentialPersistence{configPath: configPath, patStore: patStore, lookup: lookup}
}

// Resolve supplies a fresh local logout target without loading PAT values.
func (s *CredentialPersistence) Resolve(_ context.Context, alias string) (LogoutTarget, error) {
	if s == nil || s.lookup == nil {
		return LogoutTarget{}, errors.New("credential environment lookup is not configured")
	}
	configuration, err := config.Load(s.configPath)
	if err != nil {
		return LogoutTarget{}, err
	}
	environment, err := configuration.ResolveWriteEnvironment(alias)
	if err != nil {
		return LogoutTarget{}, err
	}
	name, namePresent := s.lookup.LookupEnv(environment.Auth.PATNameEnv)
	secret, secretPresent := s.lookup.LookupEnv(environment.Auth.PATSecretEnv)
	return LogoutTarget{
		Environment:                      environment.Alias,
		StoredCredentialReferencePresent: environment.Auth.CredentialRef != "",
		EnvironmentCredentialsAvailable:  namePresent && strings.TrimSpace(name) != "" && secretPresent && strings.TrimSpace(secret) != "",
	}, nil
}

// Store rechecks the authenticated target under the configuration lock.
func (s *CredentialPersistence) Store(ctx context.Context, target LoginTarget, credential LoginCredential) (LoginStoreResult, error) {
	if s == nil || s.patStore == nil || s.lookup == nil {
		return LoginStoreResult{}, errors.New("native credential storage is not configured")
	}
	var environmentVariablesOverride bool
	_, err := config.UpdateWithRollback(s.configPath, false, func(configuration config.Config) (config.Config, func() error, error) {
		environment, exists := configuration.Environments[target.Environment]
		if !exists {
			return config.Config{}, nil, fmt.Errorf("environment %q does not exist", target.Environment)
		}
		if environment.URL != target.ServerURL || environment.SiteContentURL != target.SiteContentURL {
			return config.Config{}, nil, errors.New("environment target changed after PAT validation")
		}
		resolved, resolveErr := configuration.ResolveEnvironment(target.Environment)
		if resolveErr != nil {
			return config.Config{}, nil, resolveErr
		}
		name, namePresent := s.lookup.LookupEnv(resolved.Auth.PATNameEnv)
		secret, secretPresent := s.lookup.LookupEnv(resolved.Auth.PATSecretEnv)
		environmentVariablesOverride = namePresent && strings.TrimSpace(name) != "" && secretPresent && strings.TrimSpace(secret) != ""
		if environment.Auth.CredentialRef != "" {
			reference := coreauth.CredentialReference(environment.Auth.CredentialRef)
			if err := s.patStore.ReplacePAT(ctx, reference, coreauth.CredentialTarget{ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL}, credential.PATName, credential.PATSecret); err != nil {
				return config.Config{}, nil, err
			}
			return configuration, nil, config.ErrNoChange
		}
		stored, err := s.patStore.StorePAT(ctx, coreauth.CredentialTarget{ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL}, credential.PATName, credential.PATSecret)
		if err != nil {
			return config.Config{}, nil, err
		}
		environment.Auth.CredentialRef = string(stored)
		configuration.Environments[target.Environment] = environment
		return configuration, func() error { return s.patStore.DeletePAT(context.WithoutCancel(ctx), stored) }, nil
	})
	if err != nil {
		return LoginStoreResult{}, err
	}
	return LoginStoreResult{EnvironmentVariablesOverride: environmentVariablesOverride}, nil
}

// Remove clears the selected reference before deleting its native credential.
func (s *CredentialPersistence) Remove(ctx context.Context, target LogoutTarget) (LogoutRemoveResult, error) {
	if s == nil || s.patStore == nil {
		return LogoutRemoveResult{}, errors.New("native credential storage is not configured")
	}
	removed := false
	var reference coreauth.CredentialReference
	_, err := config.UpdateWithPostSave(s.configPath, false, func(configuration config.Config) (config.Config, func() error, error) {
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
			deleteErr := s.patStore.DeletePAT(ctx, reference)
			if credentialNotFound(deleteErr) {
				return nil
			}
			return deleteErr
		}, nil
	})
	if err != nil {
		return LogoutRemoveResult{}, orphanedCredentialError(err, reference)
	}
	return LogoutRemoveResult{Removed: removed}, nil
}

// orphanedCredentialError names a recoverable entry only after its reference was cleared.
func orphanedCredentialError(err error, reference coreauth.CredentialReference) error {
	installed, installedOK := errors.AsType[*config.InstalledError](err)
	restored, restoredOK := errors.AsType[*config.PostSaveRestoreError](err)
	if !(installedOK && !installed.ExternalCommitConfirmed() || restoredOK && restored.ConfigurationInstalled()) {
		return err
	}
	return &errs.Error{Kind: errs.KindOperation, Summary: "The credential reference was cleared, but stored PAT deletion was not confirmed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the OS credential store entry " + coreauth.CredentialStoreEntry(reference) + " and remove it if present; logout no longer references it."}
}

func credentialNotFound(err error) bool {
	storeError, ok := errors.AsType[*coreauth.CredentialStoreError](err)
	return ok && storeError.Kind == coreauth.CredentialStoreNotFound
}
