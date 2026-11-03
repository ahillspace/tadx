package app

import (
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

func TestMutationConsentReadsFreshPolicyForPinnedInvocationTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := config.Environment{URL: "https://tableau.example.test", SiteContentURL: "original", Auth: config.Auth{Type: "pat"}}
	cfg := config.Config{Version: config.CurrentVersion, DefaultEnvironment: "test", Environments: map[string]config.Environment{"test": original}}
	if err := cfg.SetMutationSetting(original, true); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	runtime := &runtimeDependencies{configPath: path}
	if setting, err := runtime.ReadMutationSetting(t.Context(), ""); err != nil || !setting.Enabled {
		t.Fatalf("initial consent = %+v, %v", setting, err)
	}
	_, err := config.Update(path, false, func(current config.Config) (config.Config, error) {
		if err := current.SetMutationSetting(original, false); err != nil {
			return current, err
		}
		replacement := original
		replacement.URL = "https://replacement.example.test"
		replacement.SiteContentURL = "replacement"
		current.Environments["test"] = replacement
		return current, current.SetMutationSetting(replacement, true)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"", "test"} {
		setting, err := runtime.ReadMutationSetting(t.Context(), alias)
		if err != nil || setting.Enabled || setting.ServerURL != original.URL || setting.SiteContentURL != original.SiteContentURL {
			t.Fatalf("consent followed a retargeted alias instead of the authenticated selection: %+v, %v", setting, err)
		}
	}
}
