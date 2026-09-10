package app_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/toon"
)

func TestUsabilityUnknownEnvironmentExplainsAliasBeforeAuthentication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "team config.yaml")
	cfg := config.Config{Version: 1, Environments: map[string]config.Environment{
		"work": {URL: "https://example.invalid", SiteContentURL: "site-content-url", Auth: config.Auth{Type: "pat"}},
	}}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := app.Run(context.Background(), []string{"auth", "check", "--env", "site-content-url", "--config", path}, &out, app.Options{ConfigPath: path})
	for _, expected := range []string{"configured environment alias", "work", "env list", "--config", "prerequisite"} {
		if code == 0 || !strings.Contains(out.String(), expected) {
			t.Fatalf("missing %q: code=%d %s", expected, code, out.String())
		}
	}
	if strings.Contains(out.String(), "PAT credentials") {
		t.Fatal(out.String())
	}
}

func TestUsabilitySuggestionsPreserveExplicitConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "team's config.yaml")
	if err := config.Save(path, config.Config{Version: 1}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := app.Run(context.Background(), []string{"workspace", "create", "sample", "--path", filepath.Join(t.TempDir(), "workspace"), "--config", path}, &out, app.Options{ConfigPath: path})
	want := commandhint.Command("--config", path) + " workspace status --workspace sample"
	decoded, err := toon.Decode(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	result, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output %T", decoded)
	}
	help, ok := result["help"].([]any)
	if code != 0 || !ok || len(help) != 1 || help[0] != want {
		t.Fatalf("code=%d %s want %s", code, out.String(), want)
	}
}

func TestUsabilitySyntaxErrorsIncludeAvailableSyntax(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		expected string
	}{
		{[]string{"search", "--query", "sales"}, "search [term]"},
		{[]string{"workspace", "artifact", "list", "--workspace", "sample"}, "workspace status"},
	} {
		var out bytes.Buffer
		code := app.Run(context.Background(), tc.args, &out, app.Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")})
		if code == 0 || !strings.Contains(out.String(), tc.expected) {
			t.Fatalf("%v code=%d %s", tc.args, code, out.String())
		}
	}
}
