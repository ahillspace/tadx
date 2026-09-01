package app

import (
	"context"
	"fmt"
	"os"
	"sort"

	authstatus "github.com/ahillspace/tadx/actions/auth/status"
	profileadd "github.com/ahillspace/tadx/actions/env/profile/add"
	profileget "github.com/ahillspace/tadx/actions/env/profile/get"
	profilelist "github.com/ahillspace/tadx/actions/env/profile/list"
	profileremove "github.com/ahillspace/tadx/actions/env/profile/remove"
	profilesetdefault "github.com/ahillspace/tadx/actions/env/profile/setdefault"
	profileupdate "github.com/ahillspace/tadx/actions/env/profile/update"
	envcli "github.com/ahillspace/tadx/internal/cli/env"
	"github.com/ahillspace/tadx/internal/config"
)

type environmentCommands struct {
	list       *profilelist.Action
	get        *profileget.Action
	add        *profileadd.Action
	update     *profileupdate.Action
	remove     *profileremove.Action
	setDefault *profilesetdefault.Action
}

func newEnvironmentCommands(runtime *runtimeDependencies) *environmentCommands {
	store := configProfileStore{path: &runtime.configPath}
	return &environmentCommands{
		list: profilelist.New(store), get: profileget.New(store), add: profileadd.New(store),
		update: profileupdate.New(store), remove: profileremove.New(store), setDefault: profilesetdefault.New(store),
	}
}

func (c *environmentCommands) dependencies() *envcli.Dependencies {
	return &envcli.Dependencies{
		Lister: c, Getter: c, Adder: c, Updater: c, Remover: c, DefaultSetter: c,
		Uses:   registryUses("env.profile.list", "env.profile.get", "env.profile.add", "env.profile.update", "env.profile.remove", "env.profile.set-default"),
		Shorts: registryShorts("env.profile.list", "env.profile.get", "env.profile.add", "env.profile.update", "env.profile.remove", "env.profile.set-default"),
	}
}

func (c *environmentCommands) List(ctx context.Context, input profilelist.Input) (profilelist.Output, error) {
	return c.list.Execute(ctx, input)
}
func (c *environmentCommands) Get(ctx context.Context, input profileget.Input) (profileget.Output, error) {
	return c.get.Execute(ctx, input)
}
func (c *environmentCommands) Add(ctx context.Context, input profileadd.Input) (profileadd.Output, error) {
	return c.add.Execute(ctx, input)
}
func (c *environmentCommands) Update(ctx context.Context, input profileupdate.Input) (profileupdate.Output, error) {
	return c.update.Execute(ctx, input)
}
func (c *environmentCommands) Remove(ctx context.Context, input profileremove.Input) (profileremove.Output, error) {
	return c.remove.Execute(ctx, input)
}
func (c *environmentCommands) SetDefault(ctx context.Context, input profilesetdefault.Input) (profilesetdefault.Output, error) {
	return c.setDefault.Execute(ctx, input)
}

type configProfileStore struct{ path *string }

func (s configProfileStore) List(_ context.Context) ([]profilelist.Profile, error) {
	configuration, err := config.Load(*s.path)
	if err != nil {
		return nil, err
	}
	aliases := make([]string, 0, len(configuration.Environments))
	for alias := range configuration.Environments {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	profiles := make([]profilelist.Profile, 0, len(aliases))
	for _, alias := range aliases {
		environment, resolveErr := configuration.ResolveEnvironment(alias)
		if resolveErr != nil {
			return nil, resolveErr
		}
		profiles = append(profiles, listProfile(configuration, environment))
	}
	return profiles, nil
}

func (s configProfileStore) Get(_ context.Context, alias string) (profileget.Profile, error) {
	configuration, environment, err := s.resolve(alias)
	if err != nil {
		return profileget.Profile{}, err
	}
	return profileget.Profile{Alias: environment.Alias, Default: environment.Alias == configuration.DefaultEnvironment, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, APIVersion: environment.APIVersion, AuthType: environment.Auth.Type, PATNameEnv: environment.Auth.PATNameEnv, PATSecretEnv: environment.Auth.PATSecretEnv, DefaultWorkspace: environment.DefaultWorkspace}, nil
}

func (s configProfileStore) Add(_ context.Context, input profileadd.Profile) (profileadd.Profile, error) {
	updated, err := config.Update(*s.path, true, func(configuration config.Config) (config.Config, error) {
		if _, exists := configuration.Environments[input.Alias]; exists {
			return config.Config{}, fmt.Errorf("environment %q already exists", input.Alias)
		}
		if configuration.Environments == nil {
			configuration.Environments = make(map[string]config.Environment)
		}
		configuration.Environments[input.Alias] = config.Environment{URL: input.ServerURL, SiteContentURL: input.SiteContentURL, APIVersion: input.APIVersion, Auth: config.Auth{Type: config.AuthTypePAT, PATNameEnv: input.PATNameEnv, PATSecretEnv: input.PATSecretEnv}, DefaultWorkspace: input.DefaultWorkspace}
		return configuration, nil
	})
	if err != nil {
		return profileadd.Profile{}, err
	}
	environment, err := updated.ResolveEnvironment(input.Alias)
	if err != nil {
		return profileadd.Profile{}, err
	}
	return addProfile(environment), nil
}

func (s configProfileStore) Update(_ context.Context, alias string, patch profileupdate.Patch) (profileupdate.UpdateResult, error) {
	var changed []string
	updated, err := config.Update(*s.path, false, func(configuration config.Config) (config.Config, error) {
		environment, exists := configuration.Environments[alias]
		if !exists {
			return config.Config{}, fmt.Errorf("environment %q does not exist", alias)
		}
		changed = make([]string, 0, 6)
		apply := func(field profileupdate.StringField, name string, target *string) {
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
		if len(changed) == 0 {
			return config.Config{}, config.ErrNoChange
		}
		configuration.Environments[alias] = environment
		return configuration, nil
	})
	if err != nil {
		return profileupdate.UpdateResult{}, err
	}
	effective, err := updated.ResolveEnvironment(alias)
	if err != nil {
		return profileupdate.UpdateResult{}, err
	}
	return profileupdate.UpdateResult{Profile: profileupdate.Profile{Alias: effective.Alias, Default: effective.Alias == updated.DefaultEnvironment, ServerURL: effective.URL, SiteContentURL: effective.SiteContentURL, APIVersion: effective.APIVersion, AuthType: effective.Auth.Type, PATNameEnv: effective.Auth.PATNameEnv, PATSecretEnv: effective.Auth.PATSecretEnv, DefaultWorkspace: effective.DefaultWorkspace}, ChangedFields: changed}, nil
}

func (s configProfileStore) Remove(_ context.Context, alias string) error {
	_, err := config.Update(*s.path, false, func(configuration config.Config) (config.Config, error) {
		if _, exists := configuration.Environments[alias]; !exists {
			return config.Config{}, fmt.Errorf("environment %q does not exist", alias)
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
	_, err := config.Update(*s.path, false, func(configuration config.Config) (config.Config, error) {
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

func listProfile(configuration config.Config, environment config.Environment) profilelist.Profile {
	return profilelist.Profile{Alias: environment.Alias, Default: environment.Alias == configuration.DefaultEnvironment, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, APIVersion: environment.APIVersion, AuthType: environment.Auth.Type, PATNameEnv: environment.Auth.PATNameEnv, PATSecretEnv: environment.Auth.PATSecretEnv, DefaultWorkspace: environment.DefaultWorkspace}
}
func addProfile(environment config.Environment) profileadd.Profile {
	return profileadd.Profile{Alias: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, APIVersion: environment.APIVersion, AuthType: environment.Auth.Type, PATNameEnv: environment.Auth.PATNameEnv, PATSecretEnv: environment.Auth.PATSecretEnv, DefaultWorkspace: environment.DefaultWorkspace}
}

type authStatusResolver struct{ runtime *runtimeDependencies }

func (r authStatusResolver) Resolve(_ context.Context, alias string) (authstatus.Target, error) {
	configuration, environment, err := r.runtime.environment(alias, false)
	if err != nil {
		return authstatus.Target{}, err
	}
	return authstatus.Target{Environment: environment.Alias, Default: environment.Alias == configuration.DefaultEnvironment, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, APIVersion: environment.APIVersion, AuthType: environment.Auth.Type, PATNameVariable: environment.Auth.PATNameEnv, PATSecretVariable: environment.Auth.PATSecretEnv, DefaultWorkspace: environment.DefaultWorkspace}, nil
}

type processEnvironment struct{}

func (processEnvironment) LookupEnv(name string) (string, bool) { return os.LookupEnv(name) }

func newAuthStatus(runtime *runtimeDependencies) *authstatus.Action {
	return authstatus.New(authStatusResolver{runtime: runtime}, processEnvironment{})
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
