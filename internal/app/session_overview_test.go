package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	sessionoverview "github.com/ahillspace/tadx/actions/session/overview"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

type overviewForbiddenTransport struct{ t *testing.T }

func (r overviewForbiddenTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.t.Fatal("overview attempted HTTP")
	return nil, fmt.Errorf("HTTP forbidden")
}

type overviewForbiddenStore struct {
	coreauth.PATStore
	t *testing.T
}

func (s overviewForbiddenStore) LoadPAT(context.Context, coreauth.CredentialReference, coreauth.CredentialTarget) (coreauth.PATCredentials, error) {
	s.t.Fatal("overview retrieved OS credentials")
	return coreauth.PATCredentials{}, fmt.Errorf("credential retrieval forbidden")
}

func overviewOptions(t *testing.T, root string) Options {
	t.Helper()
	return Options{ConfigPath: filepath.Join(root, "config.yaml"), UserHomeDir: func() (string, error) { return root, nil }, PATStore: overviewForbiddenStore{t: t}, HTTPClient: &http.Client{Transport: overviewForbiddenTransport{t}}, MutationEnvironment: func() (string, bool) { return "", false }, Stderr: &bytes.Buffer{}}
}

func overviewSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			result[relative] = "directory"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[relative] = fmt.Sprintf("%x/%d/%s", sha256.Sum256(data), info.Size(), info.ModTime())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func runOverview(t *testing.T, root string, args []string, options Options) (int, string) {
	t.Helper()
	before := overviewSnapshot(t, root)
	var output bytes.Buffer
	code := Run(context.Background(), args, &output, options)
	if after := overviewSnapshot(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("overview wrote local files\nbefore=%v\nafter=%v", before, after)
	}
	return code, output.String()
}

func TestBareOverviewFirstRunAndHelpAreReadOnly(t *testing.T) {
	root := t.TempDir()
	options := overviewOptions(t, root)
	for _, args := range [][]string{nil, {"--json"}, {"--full"}, {"--full", "--json"}} {
		code, text := runOverview(t, root, args, options)
		if code != 0 || !strings.Contains(text, "local_overview") || !strings.Contains(text, "missing") || !strings.Contains(text, "not_checked") {
			t.Fatalf("args=%v code=%d output=%s", args, code, text)
		}
		if len(args) > 0 && args[len(args)-1] == "--json" && !json.Valid([]byte(text)) {
			t.Fatalf("not JSON: %s", text)
		}
	}
	code, text := runOverview(t, root, []string{"--help"}, options)
	if code != 0 || !strings.Contains(text, "discover and manage Tableau:") || !strings.Contains(text, "setup and diagnostics:") || strings.Contains(text, "local_overview") {
		t.Fatalf("help changed: code=%d output=%s", code, text)
	}
}

func TestBareOverviewShowsConfiguredNotVerifiedAndEffectiveSources(t *testing.T) {
	root := t.TempDir()
	options := overviewOptions(t, root)
	t.Setenv("TADX_OVERVIEW_NAME", "do-not-display-name")
	t.Setenv("TADX_OVERVIEW_SECRET", "do-not-display-secret")
	t.Setenv("TADX_STORE_NAME", "")
	t.Setenv("TADX_STORE_SECRET", "")
	saved := true
	cfg := config.Config{Version: config.CurrentVersion, DefaultEnvironment: "dev", MutationsEnabled: &saved, Environments: map[string]config.Environment{
		"dev":    {URL: "https://tableau.example.test", SiteContentURL: "development", Auth: config.Auth{Type: config.AuthTypePAT, PATNameEnv: "TADX_OVERVIEW_NAME", PATSecretEnv: "TADX_OVERVIEW_SECRET"}},
		"stored": {URL: "https://other.example.test", SiteContentURL: "test", Auth: config.Auth{Type: config.AuthTypePAT, PATNameEnv: "TADX_STORE_NAME", PATSecretEnv: "TADX_STORE_SECRET", CredentialRef: "cred_11111111111111111111111111111111"}},
	}}
	if err := config.Save(options.ConfigPath, cfg); err != nil {
		t.Fatal(err)
	}
	options.MutationEnvironment = func() (string, bool) { return "0", true }
	code, text := runOverview(t, root, []string{"--json", "--full"}, options)
	if code != 0 {
		t.Fatalf("code=%d output=%s", code, text)
	}
	var result sessionoverview.Result[sessionoverview.Environment, sessionoverview.Workspace]
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatal(err)
	}
	if result.ReadEnvironment != "dev" || result.WriteTarget != "explicit_environment_required" || result.AuthVerification != "not_checked" || result.Mutations.Enabled || result.Mutations.Source != "process_environment" {
		t.Fatalf("result=%#v", result)
	}
	if result.Environments.Items[0].Credentials != "configured" || result.Environments.Items[1].Credentials != "stored_reference_unverified" {
		t.Fatalf("auth=%#v", result.Environments)
	}
	for _, secret := range []string{"do-not-display-name", "do-not-display-secret", "cred_11111111111111111111111111111111"} {
		if strings.Contains(text, secret) {
			t.Fatalf("overview exposed %s", secret)
		}
	}
}

func TestBareOverviewWorkspaceResolutionReasons(t *testing.T) {
	root := t.TempDir()
	options := overviewOptions(t, root)
	if err := config.Save(options.ConfigPath, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	manager := workspacecore.NewManager(options.ConfigPath, nil)
	for _, name := range []string{"general", "target", "current"} {
		if _, err := manager.Create(context.Background(), name, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Load(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultWorkspace = "general"
	cfg.Environments = map[string]config.Environment{"dev": {URL: "https://tableau.example.test", Auth: config.Auth{Type: config.AuthTypePAT}, DefaultWorkspace: "target"}}
	if err := config.Save(options.ConfigPath, cfg); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	check := func(name, reason string) {
		t.Helper()
		code, text := runOverview(t, root, []string{"--json"}, options)
		if code != 0 {
			t.Fatalf("code=%d %s", code, text)
		}
		var result sessionoverview.Result[sessionoverview.CompactEnvironment, sessionoverview.CompactWorkspace]
		if err := json.Unmarshal([]byte(text), &result); err != nil {
			t.Fatal(err)
		}
		if result.Workspace.Name != name || result.Workspace.Reason != reason || result.Workspace.Status != "ready" {
			t.Fatalf("workspace=%#v", result.Workspace)
		}
		if result.ReadEnvironment != "dev" || result.ReadSelection != "only_environment" || result.WriteTarget != "only_environment" {
			t.Fatalf("single environment inference=%#v", result)
		}
	}
	check("target", "environment_default")
	env := cfg.Environments["dev"]
	env.DefaultWorkspace = ""
	cfg.Environments["dev"] = env
	if err := config.Save(options.ConfigPath, cfg); err != nil {
		t.Fatal(err)
	}
	check("general", "global_default")
	t.Chdir(filepath.Join(root, "current"))
	check("current", "containing_directory")
}

func TestBareOverviewMalformedConfigurationDoesNotEchoValuesOrWrite(t *testing.T) {
	root := t.TempDir()
	options := overviewOptions(t, root)
	if err := os.WriteFile(options.ConfigPath, []byte("version: pasted-private-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"--full", "--json"}} {
		code, text := runOverview(t, root, args, options)
		if code == 0 || !strings.Contains(text, "session.overview.configuration") || strings.Contains(text, "pasted-private-value") {
			t.Fatalf("code=%d output=%s", code, text)
		}
	}
}

func TestBareOverviewUnavailableWorkspaceAndIncompleteCredentials(t *testing.T) {
	root := t.TempDir()
	options := overviewOptions(t, root)
	t.Setenv("TADX_OVERVIEW_NAME", "private-name")
	t.Setenv("TADX_OVERVIEW_SECRET", "")
	configuration := config.Config{Version: config.CurrentVersion, DefaultWorkspace: "offline", Workspaces: map[string]config.WorkspaceRegistration{
		"offline": {ID: "ws_11111111111111111111111111111111", Path: filepath.Join(root, "unavailable")},
	}, Environments: map[string]config.Environment{
		"dev": {URL: "https://tableau.example.test", Auth: config.Auth{Type: config.AuthTypePAT, PATNameEnv: "TADX_OVERVIEW_NAME", PATSecretEnv: "TADX_OVERVIEW_SECRET", CredentialRef: "cred_11111111111111111111111111111111"}},
	}}
	if err := config.Save(options.ConfigPath, configuration); err != nil {
		t.Fatal(err)
	}
	code, text := runOverview(t, root, []string{"--json"}, options)
	if code != 0 {
		t.Fatalf("code=%d output=%s", code, text)
	}
	var result sessionoverview.Result[sessionoverview.CompactEnvironment, sessionoverview.CompactWorkspace]
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatal(err)
	}
	if result.Workspace.Name != "offline" || result.Workspace.Status != "unavailable" || result.Workspace.Reason != "global_default" {
		t.Fatalf("workspace=%#v", result.Workspace)
	}
	if result.Environments.Items[0].CredentialSource != "environment" || result.Environments.Items[0].Credentials != "incomplete" {
		t.Fatalf("incomplete environment credentials should not silently fall back to store: %#v", result.Environments)
	}
	if strings.Contains(text, "private-name") {
		t.Fatal("printed credential")
	}
}

func TestBareOverviewDoesNotMigrateLegacyLocalFiles(t *testing.T) {
	root := t.TempDir()
	options := overviewOptions(t, root)
	workspaceRoot := filepath.Join(root, "legacy")
	if err := os.MkdirAll(workspaceRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "tadx.yaml"), []byte("version: 1\nworkspace:\n  name: legacy\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data := "version: 1\ndefault_workspace: " + filepath.ToSlash(workspaceRoot) + "\n"
	if err := os.WriteFile(options.ConfigPath, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	code, text := runOverview(t, root, []string{"--json", "--full"}, options)
	if code != 0 || !strings.Contains(text, "local_overview") {
		t.Fatalf("code=%d output=%s", code, text)
	}
}
