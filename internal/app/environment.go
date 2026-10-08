package app

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"

	authops "github.com/ahillspace/tadx/actions/auth"
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
	aliases := configuration.EnvironmentAliases()
	profiles := make([]profile.Profile, 0, len(aliases))
	for _, alias := range aliases {
		if invalid, ok := configuration.InvalidEnvironments[alias]; ok {
			profiles = append(profiles, invalidProfile(configuration, alias, invalid))
			continue
		}
		environment, resolveErr := configuration.ResolveEnvironment(alias)
		if resolveErr != nil {
			return nil, resolveErr
		}
		profiles = append(profiles, profileFromConfig(configuration, environment))
	}
	return profiles, nil
}

func (s configProfileStore) Get(_ context.Context, alias string) (profile.Profile, error) {
	configuration, err := config.Load(*s.path)
	if err != nil {
		return profile.Profile{}, err
	}
	if invalid, ok := configuration.InvalidEnvironments[alias]; ok {
		return invalidProfile(configuration, alias, invalid), nil
	}
	if _, ok := configuration.Environments[alias]; !ok {
		return profile.Profile{}, fmt.Errorf("environment %q does not exist", alias)
	}
	environment, err := configuration.ResolveEnvironment(alias)
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
		if slices.Contains(configuration.EnvironmentAliases(), input.Alias) {
			return config.Config{}, fmt.Errorf("environment %q already exists", input.Alias)
		}
		if configuration.Environments == nil {
			configuration.Environments = make(map[string]config.Environment)
		}
		configuration.Environments[input.Alias] = config.Environment{URL: input.ServerURL, SiteContentURL: input.SiteContentURL, Auth: config.Auth{Type: config.AuthTypePAT, PATNameEnv: input.PATNameEnv, PATSecretEnv: input.PATSecretEnv}, DefaultWorkspace: input.DefaultWorkspace, CacheMaxConcurrency: input.CacheMaxConcurrency}
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
	profile.MultipleEnvironments = len(updated.EnvironmentAliases()) == 2
	return profile, nil
}

func (s configProfileStore) Update(_ context.Context, alias string, patch profile.Patch) (profile.UpdateResult, error) {
	var changed []string
	updated, err := s.updateConfig(false, func(configuration config.Config) (config.Config, error) {
		environment, err := configuration.EnvironmentForRepair(alias)
		if err != nil {
			return config.Config{}, err
		}
		invalid, quarantined := configuration.InvalidEnvironments[alias]
		credentialRef := environment.Auth.CredentialRef
		if quarantined {
			credentialRef = invalid.CredentialRef
		}
		if credentialRef != "" &&
			((patch.ServerURL.Set && environment.URL != patch.ServerURL.Value) ||
				(patch.SiteContentURL.Set && environment.SiteContentURL != patch.SiteContentURL.Value)) {
			return config.Config{}, &storedPATProfileError{alias: alias, action: "changing its Tableau target"}
		}
		changed = make([]string, 0, 6)
		fields := make(map[string]any)
		apply := func(field profile.StringField, name, path, current string) {
			if field.Set && (quarantined || current != field.Value) {
				fields[path] = field.Value
				changed = append(changed, name)
			}
		}
		apply(patch.ServerURL, "server_url", "url", environment.URL)
		apply(patch.SiteContentURL, "site_content_url", "site_content_url", environment.SiteContentURL)
		apply(patch.PATNameEnv, "pat_name_env", "auth.pat_name_env", environment.Auth.PATNameEnv)
		apply(patch.PATSecretEnv, "pat_secret_env", "auth.pat_secret_env", environment.Auth.PATSecretEnv)
		apply(patch.DefaultWorkspace, "default_workspace", "default_workspace", environment.DefaultWorkspace)
		if field := patch.CacheMaxConcurrency; field.Set && (quarantined || environment.CacheMaxConcurrency != field.Value) {
			fields["cache_max_concurrency"] = field.Value
			changed = append(changed, "cache_max_concurrency")
		}
		if len(changed) == 0 {
			return config.Config{}, config.ErrNoChange
		}
		if err := configuration.PatchEnvironment(alias, fields); err != nil {
			return config.Config{}, err
		}
		return configuration, nil
	})
	if err != nil {
		return profile.UpdateResult{}, err
	}
	effective, err := updated.ResolveEnvironment(alias)
	if err != nil {
		return profile.UpdateResult{}, err
	}
	return profile.UpdateResult{Profile: profile.UpdateProfile{Alias: effective.Alias, Default: effective.Alias == updated.DefaultEnvironment, ServerURL: effective.URL, SiteContentURL: effective.SiteContentURL, AuthType: effective.Auth.Type, PATNameEnv: effective.Auth.PATNameEnv, PATSecretEnv: effective.Auth.PATSecretEnv, DefaultWorkspace: effective.DefaultWorkspace, CacheMaxConcurrency: effective.CacheMaxConcurrency}, ChangedFields: changed}, nil
}

func (s configProfileStore) Remove(_ context.Context, alias string) error {
	_, err := s.updateConfig(false, func(configuration config.Config) (config.Config, error) {
		environment, err := configuration.EnvironmentForRepair(alias)
		if err != nil {
			return config.Config{}, err
		}
		invalid, quarantined := configuration.InvalidEnvironments[alias]
		credentialRef := environment.Auth.CredentialRef
		if quarantined {
			credentialRef = invalid.CredentialRef
		}
		if credentialRef != "" {
			return config.Config{}, &storedPATProfileError{alias: alias, action: "removing it"}
		}
		if configuration.DefaultEnvironment == alias {
			if !quarantined {
				return config.Config{}, fmt.Errorf("environment %q is the default and cannot be removed", alias)
			}
			configuration.DefaultEnvironment = ""
		}
		if err := configuration.RemoveEnvironment(alias); err != nil {
			return config.Config{}, err
		}
		return configuration, nil
	})
	return err
}

type storedPATProfileError struct{ alias, action string }

func (e *storedPATProfileError) Error() string {
	return fmt.Sprintf("environment %q has a stored PAT; log out before %s", e.alias, e.action)
}
func (*storedPATProfileError) Retryable() bool { return false }
func (e *storedPATProfileError) CorrectiveAction() string {
	return "Log out of the selected environment before " + e.action + "."
}
func (e *storedPATProfileError) CorrectiveCommands() [][]string {
	return [][]string{{"auth", "logout", "--environment", e.alias}}
}
func (e *storedPATProfileError) CorrectiveExplanation() string {
	return "Log out before " + e.action + "."
}

func (s configProfileStore) SetDefault(_ context.Context, alias string) (bool, error) {
	changed := false
	_, err := s.updateConfig(false, func(configuration config.Config) (config.Config, error) {
		if _, err := configuration.ResolveEnvironment(alias); err != nil {
			return config.Config{}, err
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

func invalidProfile(configuration config.Config, alias string, invalid config.InvalidEntry) profile.Profile {
	fields := make([]string, 0, len(invalid.Violations))
	for _, violation := range invalid.Violations {
		fields = append(fields, violation.Field)
	}
	slices.Sort(fields)
	return profile.Profile{Alias: alias, Default: alias == configuration.DefaultEnvironment, Status: "invalid", Violations: slices.Compact(fields)}
}

func profileFromConfig(configuration config.Config, environment config.Environment) profile.Profile {
	return profile.Profile{Alias: environment.Alias, Default: environment.Alias == configuration.DefaultEnvironment, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, AuthType: environment.Auth.Type, PATNameEnv: environment.Auth.PATNameEnv, PATSecretEnv: environment.Auth.PATSecretEnv, DefaultWorkspace: environment.DefaultWorkspace, CacheMaxConcurrency: environment.CacheMaxConcurrency}
}
func addProfile(environment config.Environment) profile.AddProfile {
	return profile.AddProfile{Alias: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, AuthType: environment.Auth.Type, PATNameEnv: environment.Auth.PATNameEnv, PATSecretEnv: environment.Auth.PATSecretEnv, DefaultWorkspace: environment.DefaultWorkspace, CacheMaxConcurrency: environment.CacheMaxConcurrency}
}

type authStatusResolver struct{ runtime *runtimeDependencies }

func (r authStatusResolver) Resolve(_ context.Context, alias string) (authops.StatusTarget, error) {
	configuration, environment, err := r.runtime.environment(alias, false)
	if err != nil {
		return authops.StatusTarget{}, err
	}
	return authStatusTarget(configuration, environment), nil
}

func authStatusTarget(configuration config.Config, environment config.Environment) authops.StatusTarget {
	return authops.StatusTarget{Environment: environment.Alias, Default: environment.Alias == configuration.DefaultEnvironment, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, AuthType: environment.Auth.Type, PATNameVariable: environment.Auth.PATNameEnv, PATSecretVariable: environment.Auth.PATSecretEnv, StoredCredentialReferencePresent: environment.Auth.CredentialRef != "", DefaultWorkspace: environment.DefaultWorkspace}
}

type processEnvironment struct{}

func (processEnvironment) LookupEnv(name string) (string, bool) { return os.LookupEnv(name) }

func newAuthStatus(runtime *runtimeDependencies) *authops.StatusAction {
	return authops.NewStatus(authStatusResolver{runtime: runtime}, processEnvironment{})
}

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
