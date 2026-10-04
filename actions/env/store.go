package env

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/ahillspace/tadx/internal/config"
)

type configStore struct {
	path                       func() string
	defaultCacheMaxConcurrency int
	preview                    bool
}

// NewConfigStore binds profile operations to one selected settings file.
func NewConfigStore(path func() string, defaultCacheMaxConcurrency int) Store {
	return configStore{path: path, defaultCacheMaxConcurrency: defaultCacheMaxConcurrency}
}

func (s configStore) updateConfig(createIfMissing bool, mutate func(config.Config) (config.Config, error)) (config.Config, error) {
	if s.preview {
		return config.PreviewUpdate(s.path(), createIfMissing, mutate)
	}
	return config.Update(s.path(), createIfMissing, mutate)
}

func (s configStore) PreviewAdd(ctx context.Context, input AddProfile) (AddProfile, error) {
	s.preview = true
	return s.Add(ctx, input)
}
func (s configStore) PreviewUpdate(ctx context.Context, alias string, patch Patch) (UpdateResult, error) {
	s.preview = true
	return s.Update(ctx, alias, patch)
}
func (s configStore) PreviewRemove(ctx context.Context, alias string) error {
	s.preview = true
	return s.Remove(ctx, alias)
}
func (s configStore) PreviewSetDefault(ctx context.Context, alias string) (bool, error) {
	s.preview = true
	return s.SetDefault(ctx, alias)
}

func (s configStore) List(_ context.Context) ([]Profile, error) {
	configuration, err := config.Load(s.path())
	if err != nil {
		return nil, err
	}
	aliases := make([]string, 0, len(configuration.Environments))
	for alias := range configuration.Environments {
		aliases = append(aliases, alias)
	}
	slices.Sort(aliases)
	profiles := make([]Profile, 0, len(aliases))
	for _, alias := range aliases {
		environment, resolveErr := configuration.ResolveEnvironment(alias)
		if resolveErr != nil {
			return nil, resolveErr
		}
		profiles = append(profiles, profileFromConfig(configuration, environment))
	}
	return profiles, nil
}

func (s configStore) Get(_ context.Context, alias string) (Profile, error) {
	configuration, environment, err := s.resolve(alias)
	if err != nil {
		return Profile{}, err
	}
	result := profileFromConfig(configuration, environment)
	result.DefaultWorkspace = cmp.Or(environment.DefaultWorkspace, configuration.DefaultWorkspace)
	result.CacheMaxConcurrency = cmp.Or(environment.CacheMaxConcurrency, s.defaultCacheMaxConcurrency)
	return result, nil
}

func (s configStore) Add(_ context.Context, input AddProfile) (AddProfile, error) {
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
		return AddProfile{}, err
	}
	environment, err := updated.ResolveEnvironment(input.Alias)
	if err != nil {
		return AddProfile{}, err
	}
	profile := addProfile(environment)
	profile.MultipleEnvironments = len(updated.Environments) == 2
	return profile, nil
}

func (s configStore) Update(_ context.Context, alias string, patch Patch) (UpdateResult, error) {
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
		apply := func(field StringField, name string, target *string) {
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
		return UpdateResult{}, err
	}
	effective, err := updated.ResolveEnvironment(alias)
	if err != nil {
		return UpdateResult{}, err
	}
	return UpdateResult{Profile: UpdateProfile{Alias: effective.Alias, Default: effective.Alias == updated.DefaultEnvironment, ServerURL: effective.URL, SiteContentURL: effective.SiteContentURL, APIVersion: effective.APIVersion, AuthType: effective.Auth.Type, PATNameEnv: effective.Auth.PATNameEnv, PATSecretEnv: effective.Auth.PATSecretEnv, DefaultWorkspace: effective.DefaultWorkspace, CacheMaxConcurrency: effective.CacheMaxConcurrency}, ChangedFields: changed}, nil
}

func (s configStore) Remove(_ context.Context, alias string) error {
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

func (s configStore) SetDefault(_ context.Context, alias string) (bool, error) {
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

func (s configStore) resolve(alias string) (config.Config, config.Environment, error) {
	configuration, err := config.Load(s.path())
	if err != nil {
		return config.Config{}, config.Environment{}, err
	}
	environment, err := configuration.ResolveEnvironment(alias)
	return configuration, environment, err
}

func profileFromConfig(configuration config.Config, environment config.Environment) Profile {
	return Profile{Alias: environment.Alias, Default: environment.Alias == configuration.DefaultEnvironment, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, APIVersion: environment.APIVersion, AuthType: environment.Auth.Type, PATNameEnv: environment.Auth.PATNameEnv, PATSecretEnv: environment.Auth.PATSecretEnv, DefaultWorkspace: environment.DefaultWorkspace, CacheMaxConcurrency: environment.CacheMaxConcurrency}
}
func addProfile(environment config.Environment) AddProfile {
	return AddProfile{Alias: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, APIVersion: environment.APIVersion, AuthType: environment.Auth.Type, PATNameEnv: environment.Auth.PATNameEnv, PATSecretEnv: environment.Auth.PATSecretEnv, DefaultWorkspace: environment.DefaultWorkspace, CacheMaxConcurrency: environment.CacheMaxConcurrency}
}
