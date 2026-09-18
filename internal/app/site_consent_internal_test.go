package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

// withSiteMutationConsent saves consent in an isolated fixture configuration.
// Disabled variants copy the configuration to preserve the enabled fixture.
func withSiteMutationConsent(t *testing.T, options Options, enabled bool) Options {
	t.Helper()
	cfg := config.Config{Version: config.CurrentVersion}
	if options.ConfigPath != "" {
		var err error
		cfg, err = config.Load(options.ConfigPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	if cfg.Version == 0 {
		cfg.Version = config.CurrentVersion
	}
	if len(cfg.Environments) == 0 {
		cfg.Environments = map[string]config.Environment{"production": {URL: "https://tableau.example.test", Auth: config.Auth{Type: config.AuthTypePAT}}}
	}
	for _, environment := range cfg.Environments {
		if err := cfg.SetMutationSetting(environment, enabled); err != nil {
			t.Fatal(err)
		}
	}
	if options.ConfigPath == "" || !enabled {
		options.ConfigPath = filepath.Join(t.TempDir(), "site-consent.yaml")
	}
	if err := config.Save(options.ConfigPath, cfg); err != nil {
		t.Fatal(err)
	}
	return options
}
