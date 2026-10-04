package app

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"

	authstatus "github.com/ahillspace/tadx/actions/auth"
	"github.com/ahillspace/tadx/actions/env/profile"
	envcli "github.com/ahillspace/tadx/internal/cli/env"
	"github.com/ahillspace/tadx/internal/config"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

func newEnvironmentDependencies(runtime *runtimeDependencies) *envcli.Dependencies {
	store := configProfileStore{path: &runtime.configPath}
	return &envcli.Dependencies{
		Lister: profile.NewList(store).Execute, Getter: profile.NewGet(store).Execute, Adder: profile.NewAdd(store).Execute,
		Updater: profile.NewUpdate(store).Execute, Remover: profile.NewRemove(store).Execute, DefaultSetter: profile.NewSetDefault(store).Execute,
		Uses:   registryUses("env.profile.list", "env.profile.get", "env.profile.add", "env.profile.update", "env.profile.remove", "env.profile.set-default"),
		Shorts: registryShorts("env.profile.list", "env.profile.get", "env.profile.add", "env.profile.update", "env.profile.remove", "env.profile.set-default"),
	}
}

type configProfileStore struct {
	path    *string
	preview bool
}

func (s configProfileStore) updateConfig(createIfMissing bool, mutate func(config.Config) (config.Config, error)) (config.Config, error) {
	if s.preview {
		return config.PreviewUpdate(*s.path, createIfMissing, mutate)
	}
	return config.Update(*s.path, createIfMissing, mutate)
}

func (s configProfileStore) PreviewAdd(ctx context.Context, input profile.AddProfile) (profile.AddProfile, error) {
	s.preview = true
	return s.Add(ctx, input)
}
func (s configProfileStore) PreviewUpdate(ctx context.Context, alias string, patch profile.Patch) (profile.UpdateResult, error) {
	s.preview = true
	return s.Update(ctx, alias, patch)
}
func (s configProfileStore) PreviewRemove(ctx context.Context, alias string) error {
	s.preview = true
	return s.Remove(ctx, alias)
}
func (s configProfileStore) PreviewSetDefault(ctx context.Context, alias string) (bool, error) {
	s.preview = true
	return s.SetDefault(ctx, alias)
}

func (s configProfileStore) List(_ context.Context) ([]profile.Profile, error) {
	configuration, err := config.Load(*s.path)
	if err != nil {
		return nil, err
	}
	aliases := make([]string, 0, len(configuration.Environments))
	for alias := range configuration.Environments {
		aliases = append(aliases, alias)
	}
	slices.Sort(aliases)
	profiles := make([]profile.Profile, 0, len(aliases))
	for _, alias := range aliases {
		environment, resolveErr := configuration.ResolveEnvironment(alias)
		if resolveErr != nil {
			return nil, resolveErr
		}
		profiles = append(profiles, profileFromConfig(configuration, environment))
	}
	return profiles, nil
}

func (s configProfileStore) Get(_ context.Context, alias string) (profile.Profile, error) {
	configuration, environment, err := s.resolve(alias)
	if err != nil {
		return profile.Profile{}, err
	}
	result := profileFromConfig(configuration, environment)
	result.DefaultWorkspace = cmp.Or(environment.DefaultWorkspace, configuration.DefaultWorkspace)
	result.CacheMaxConcurrency = cmp.Or(environment.CacheMaxConcurrency, tableaucache.DefaultMaxConcurrency)
	return result, nil
}

func (s configProfileStore) Add(_ context.Context, input profile.AddProfile) (profile.AddProfile, error) {
	updated, err := s.updateConfig(true, func(configuration config.Config) (config.Config, error) {
		if _, exists := configuration.Environments[input.Alias]; exists {
			return config.Config{}, fmt.Errorf("environment %q already exists", input.Alias)
		}
		if configuration.Environments == nil {
			configuration.Environments = make(map[string]config.Environment)
		}
		configuration.Environments[input.Alias] = config.Environment{URL: input.ServerURL, SiteContentURL: input.SiteContentURL, APIVersion: input.APIVersion, Auth: config.Auth{Type: config.AuthTypePAT, PATNameEnv: input.PATNameEnv, PATSecretEnv: input.PATSecretEnv}, DefaultWorkspace: input.DefaultWorkspace, CacheMaxConcurrency: input.CacheMaxConcurrency}
		return configuration, nil
	})
	if err != nil {
		return profile.AddProfile{}, err
	}
	environment, err := updated.ResolveEnvironment(input.Alias)
	if err != nil {
		return profile.AddProfile{}, err
	}
	profile := addProfile(environment)
	profile.MultipleEnvironments = len(updated.Environments) == 2
	return profile, nil
}

func (s configProfileStore) Update(_ context.Context, alias string, patch profile.Patch) (profile.UpdateResult, error) {
	var changed []string
	updated, err := s.updateConfig(false, func(configuration config.Config) (config.Config, error) {
		environment, exists := configuration.Environments[alias]
		if !exists {
			return config.Config{}, fmt.Errorf("environment %q does not exist", alias)
		}
		if environment.Auth.CredentialRef != "" &&
			((patch.ServerURL.Set && environment.URL != patch.ServerURL.Value) ||
				(patch.SiteContentURL.Set && environment.SiteContentURL != patch.SiteContentURL.Value)) {
			return config.Config{}, fmt.Errorf("environment %q has a stored PAT; run tadx auth logout --environment %s before changing its Tableau target", alias, alias)
		}
		changed = make([]string, 0, 6)
		apply := func(field profile.StringField, name string, target *string) {
			if field.Set && *target != field.Value {
				*target = field.Value
				changed = append(changed, name)
			}
		}
		apply(patch.ServerURL, "server_url", &environment.URL)
		apply(patch.SiteContentURL, "site_content_url", &environment.SiteContentURL)
		apply(patch.APIVersion, "api_version", &environment.APIVersion)
		apply(patch.PATNameEnv, "pat_name_env", &environment.Auth.PATNameEnv)
		apply(patch.PATSecretEnv, "pat_secret_env", &environment.Auth.PATSecretEnv)
		apply(patch.DefaultWorkspace, "default_workspace", &environment.DefaultWorkspace)
		if field := patch.CacheMaxConcurrency; field.Set && environment.CacheMaxConcurrency != field.Value {
			environment.CacheMaxConcurrency = field.Value
			changed = append(changed, "cache_max_concurrency")
		}
		if len(changed) == 0 {
			return config.Config{}, config.ErrNoChange
		}
		configuration.Environments[alias] = environment
		return configuration, nil
	})
	if err != nil {
		return profile.UpdateResult{}, err
	}
	effective, err := updated.ResolveEnvironment(alias)
	if err != nil {
		return profile.UpdateResult{}, err
	}
	return profile.UpdateResult{Profile: profile.UpdateProfile{Alias: effective.Alias, Default: effective.Alias == updated.DefaultEnvironment, ServerURL: effective.URL, SiteContentURL: effective.SiteContentURL, APIVersion: effective.APIVersion, AuthType: effective.Auth.Type, PATNameEnv: effective.Auth.PATNameEnv, PATSecretEnv: effective.Auth.PATSecretEnv, DefaultWorkspace: effective.DefaultWorkspace, CacheMaxConcurrency: effective.CacheMaxConcurrency}, ChangedFields: changed}, nil
}

func (s configProfileStore) Remove(_ context.Context, alias string) error {
	_, err := s.updateConfig(false, func(configuration config.Config) (config.Config, error) {
		environment, exists := configuration.Environments[alias]
		if !exists {
			return config.Config{}, fmt.Errorf("environment %q does not exist", alias)
		}
		if environment.Auth.CredentialRef != "" {
			return config.Config{}, fmt.Errorf("environment %q has a stored PAT; run tadx auth logout --environment %s before removing it", alias, alias)
		}
		if configuration.DefaultEnvironment == alias {
			return config.Config{}, fmt.Errorf("environment %q is the default and cannot be removed", alias)
		}
		delete(configuration.Environments, alias)
		return configuration, nil
	})
	return err
}

func (s configProfileStore) SetDefault(_ context.Context, alias string) (bool, error) {
	changed := false
	_, err := s.updateConfig(false, func(configuration config.Config) (config.Config, error) {
		if _, exists := configuration.Environments[alias]; !exists {
			return config.Config{}, fmt.Errorf("environment %q does not exist", alias)
		}
		if configuration.DefaultEnvironment == alias {
			return config.Config{}, config.ErrNoChange
		}
		configuration.DefaultEnvironment = alias
		changed = true
		return configuration, nil
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}

func (s configProfileStore) resolve(alias string) (config.Config, config.Environment, error) {
	configuration, err := config.Load(*s.path)
	if err != nil {
		return config.Config{}, config.Environment{}, err
	}
	environment, err := configuration.ResolveEnvironment(alias)
	return configuration, environment, err
}

func profileFromConfig(configuration config.Config, environment config.Environment) profile.Profile {
	return profile.Profile{Alias: environment.Alias, Default: environment.Alias == configuration.DefaultEnvironment, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, APIVersion: environment.APIVersion, AuthType: environment.Auth.Type, PATNameEnv: environment.Auth.PATNameEnv, PATSecretEnv: environment.Auth.PATSecretEnv, DefaultWorkspace: environment.DefaultWorkspace, CacheMaxConcurrency: environment.CacheMaxConcurrency}
}
func addProfile(environment config.Environment) profile.AddProfile {
	return profile.AddProfile{Alias: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, APIVersion: environment.APIVersion, AuthType: environment.Auth.Type, PATNameEnv: environment.Auth.PATNameEnv, PATSecretEnv: environment.Auth.PATSecretEnv, DefaultWorkspace: environment.DefaultWorkspace, CacheMaxConcurrency: environment.CacheMaxConcurrency}
}

type authStatusResolver struct{ runtime *runtimeDependencies }

func (r authStatusResolver) Resolve(_ context.Context, alias string) (authstatus.StatusTarget, error) {
	configuration, environment, err := r.runtime.environment(alias, false)
	if err != nil {
		return authstatus.StatusTarget{}, err
	}
	return authstatus.StatusTargetFromConfig(configuration, environment), nil
}

type processEnvironment struct{}

func (processEnvironment) LookupEnv(name string) (string, bool) { return os.LookupEnv(name) }

func registryUses(ids ...string) map[string]string {
	result := make(map[string]string, len(ids))
	for _, id := range ids {
		result[id] = registryLeafUse(id)
	}
	return result
}
func registryShorts(ids ...string) map[string]string {
	result := make(map[string]string, len(ids))
	for _, id := range ids {
		result[id] = registryShort(id)
	}
	return result
}
