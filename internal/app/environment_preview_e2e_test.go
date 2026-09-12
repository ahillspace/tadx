package app_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/config"
)

func TestEnvironmentPreviewValidatesWithoutChangingConfiguration(t *testing.T) {
	for _, test := range []struct {
		name     string
		args     []string
		succeeds bool
	}{
		{"add", []string{"add", "new", "--url", "https://new.example.test", "--site", "new-site"}, true},
		{"update", []string{"update", "other", "--site", "next"}, true},
		{"remove", []string{"remove", "other"}, true},
		{"default", []string{"default", "other"}, true},
		{"unchanged", []string{"default", "main"}, true},
		{"duplicate", []string{"add", "other", "--url", "https://new.example.test"}, false},
		{"missing", []string{"update", "absent", "--site", "next"}, false},
		{"default_guard", []string{"remove", "main"}, false},
		{"credential_remove", []string{"remove", "stored"}, false},
		{"credential_target", []string{"update", "stored", "--site", "next"}, false},
		{"effective_pat_collision", []string{"update", "other", "--pat-name-env", "OTHER_SECRET"}, false},
		{"invalid_url", []string{"add", "bad", "--url", "http://example.test"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "config.yaml")
			cfg := config.Config{Version: config.CurrentVersion, DefaultEnvironment: "main", Environments: map[string]config.Environment{
				"main":   {URL: "https://tableau.example.test", Auth: config.Auth{Type: config.AuthTypePAT}},
				"other":  {URL: "https://tableau.example.test", Auth: config.Auth{Type: config.AuthTypePAT, PATNameEnv: "OTHER_NAME", PATSecretEnv: "OTHER_SECRET"}},
				"stored": {URL: "https://tableau.example.test", Auth: config.Auth{Type: config.AuthTypePAT, CredentialRef: "cred_0123456789abcdef0123456789abcdef"}},
			}}
			if err := config.Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			before := acquisitionSnapshot(t, root)
			args := append([]string{"env"}, test.args...)
			args = append(args, "--preview")
			var out strings.Builder
			code := app.Run(context.Background(), args, &out, app.Options{ConfigPath: path})
			if (code == 0) != test.succeeds {
				t.Fatalf("exit=%d output=%s", code, out.String())
			}
			if test.succeeds && !strings.Contains(out.String(), "status: preview") {
				t.Fatalf("not a preview: %s", out.String())
			}
			after := acquisitionSnapshot(t, root)
			// Last-command output is an operational receipt, not profile state.
			delete(after, "last-result.json")
			delete(after, "last-result.json.lock")
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("preview wrote profile, lock, or cache files: before=%v after=%v", before, after)
			}
		})
	}
}

func TestEnvironmentAddPreviewMissingConfigurationAndExplicitFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "config.yaml")
	options := app.Options{ConfigPath: path}
	args := []string{"env", "add", "new", "--url", "https://tableau.example.test", "--preview"}
	runGroupOneCLI(t, options, args...)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("preview created config: %v", err)
	}
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("preview created lock: %v", err)
	}
	args[len(args)-1] = "--preview=false"
	output := runGroupOneCLI(t, options, args...)
	if !strings.Contains(output, "status: added") {
		t.Fatalf("explicit false did not execute: %s", output)
	}
	if _, err := config.Load(path); err != nil {
		t.Fatal(err)
	}
}
