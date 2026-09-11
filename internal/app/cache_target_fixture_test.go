package app

import (
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/config"
	"path/filepath"
	"testing"
	"time"
)

func targetCacheFixture(t *testing.T, configPath string, now func() time.Time) *cache.Store {
	t.Helper()
	configuration, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := configuration.ResolveEnvironment("")
	if err != nil {
		t.Fatal(err)
	}
	return cache.NewTargetStore(filepath.Dir(configPath), environment.URL, environment.SiteContentURL, now)
}
