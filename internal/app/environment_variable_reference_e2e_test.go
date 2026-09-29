package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/config"
)

func TestEnvironmentPATReferencesRejectSecretsWithoutPersistingThem(t *testing.T) {
	const secret = "abc123DEF==:ghiJKL456"
	for _, test := range []struct {
		name string
		args []string
	}{
		{"add_secret", []string{"env", "add", "probe", "--url", "https://example.test", "--site", "demo", "--pat-name-env", "TADX_PROBE_NAME", "--pat-secret-env", secret}},
		{"add_name", []string{"env", "add", "probe", "--url", "https://example.test", "--pat-name-env", secret}},
		{"add_preview", []string{"env", "add", "probe", "--url", "https://example.test", "--pat-secret-env", secret, "--preview"}},
		{"add_json", []string{"env", "add", "probe", "--url", "https://example.test", "--pat-secret-env", secret, "--json"}},
		{"update_secret", []string{"env", "update", "main", "--pat-secret-env", secret}},
		{"update_full", []string{"env", "update", "main", "--pat-name-env", secret, "--full"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "config.yaml")
			cfg := config.Config{Version: config.CurrentVersion, DefaultEnvironment: "main", Environments: map[string]config.Environment{
				"main": {URL: "https://tableau.example.test", Auth: config.Auth{Type: config.AuthTypePAT}},
			}}
			if err := config.Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			var out strings.Builder
			if code := app.Run(context.Background(), test.args, &out, app.Options{ConfigPath: path}); code == 0 {
				t.Fatalf("exit=0 output=%s", out.String())
			}
			if strings.Contains(out.String(), secret) {
				t.Fatalf("rejected reference echoed: %s", out.String())
			}
			if !strings.Contains(out.String(), "not shown because it may be a secret") {
				t.Fatalf("missing guidance: %s", out.String())
			}
			assertTreeOmits(t, root, secret)
			var last strings.Builder
			app.Run(context.Background(), []string{"last", "--full"}, &last, app.Options{ConfigPath: path})
			if strings.Contains(last.String(), secret) {
				t.Fatalf("last result echoed rejected reference: %s", last.String())
			}
		})
	}
}

func TestEnvironmentPATReferencesAcceptVariableNames(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.yaml")
	if err := config.Save(path, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	args := []string{"env", "add", "probe", "--url", "https://example.test", "--pat-name-env", "PROBE_PAT_NAME", "--pat-secret-env", "PROBE_PAT_SECRET"}
	if code := app.Run(context.Background(), args, &out, app.Options{ConfigPath: path}); code != 0 {
		t.Fatalf("exit=%d output=%s", code, out.String())
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if auth := loaded.Environments["probe"].Auth; auth.PATNameEnv != "PROBE_PAT_NAME" || auth.PATSecretEnv != "PROBE_PAT_SECRET" {
		t.Fatalf("auth = %#v", auth)
	}
}

func assertTreeOmits(t *testing.T, root, value string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), value) {
			t.Errorf("%s contains the rejected reference", filepath.Base(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
