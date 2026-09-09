package app

import (
	"github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/config"
	"path/filepath"
	"testing"
	"time"
)

func targetCatalogFixture(t *testing.T, configPath string, now func() time.Time) *catalog.Store {
	t.Helper()
	configuration, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := configuration.ResolveEnvironment("")
	if err != nil {
		t.Fatal(err)
	}
	return catalog.NewTargetStore(filepath.Dir(configPath), environment.URL, environment.SiteContentURL, now)
}
