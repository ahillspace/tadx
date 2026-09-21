package app_test

import (
	"bytes"
	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/config"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistentMutationSettingThroughCLI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.Config{Version: 1, MutationsEnabled: new(true), Environments: map[string]config.Environment{
		"a":     {URL: "https://tableau.example.com", SiteContentURL: "a", Auth: config.Auth{Type: "pat"}},
		"alias": {URL: "https://TABLEAU.example.com:443/", SiteContentURL: "a", Auth: config.Auth{Type: "pat"}},
		"b":     {URL: "https://tableau.example.com", SiteContentURL: "b", Auth: config.Auth{Type: "pat"}},
	}}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	invoke := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if code := app.Run(t.Context(), args, &out, app.Options{ConfigPath: path}); code != 0 {
			t.Fatalf("code=%d %s", code, out.String())
		}
		return out.String()
	}
	if got := invoke("mutation", "status", "--environment", "a"); !strings.Contains(got, "a,false") {
		t.Fatal(got)
	}
	if got := invoke("mutation", "set", "--environment", "a", "--enabled=true"); !strings.Contains(got, "enabled: true") || !strings.Contains(got, "persisted: true") || !strings.Contains(got, "source_setting: site_mutations") {
		t.Fatal(got)
	}
	if got := invoke("mutation", "status", "--environment", "alias", "--full"); !strings.Contains(got, "saved_site_setting") {
		t.Fatal(got)
	}
	var overridden bytes.Buffer
	if code := app.Run(t.Context(), []string{"mutation", "status", "--environment", "a"}, &overridden, app.Options{ConfigPath: path}); code != 0 || !strings.Contains(overridden.String(), "a,true") || strings.Contains(overridden.String(), "process_environment") {
		t.Fatal(overridden.String())
	}
	if got := invoke("mutation", "status", "--environment", "b"); !strings.Contains(got, "b,false") {
		t.Fatal(got)
	}
	for _, args := range [][]string{{"mutation", "set", "--enabled=true"}, {"mutation", "set", "--enabled=true", "--environment", "missing"}} {
		var out bytes.Buffer
		if code := app.Run(t.Context(), args, &out, app.Options{ConfigPath: path}); code == 0 {
			t.Fatalf("unresolved target enabled: %s", out.String())
		}
	}
	if _, err := config.Update(path, false, func(c config.Config) (config.Config, error) {
		e := c.Environments["a"]
		e.SiteContentURL = "retargeted"
		c.Environments["a"] = e
		return c, nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := invoke("mutation", "status", "--environment", "a"); !strings.Contains(got, "a,false") {
		t.Fatal(got)
	}
	invoke("mutation", "set", "--environment", "alias", "--enabled=false")
	if got := invoke("mutation", "status", "--environment", "alias"); !strings.Contains(got, "alias,false") {
		t.Fatal(got)
	}
}
