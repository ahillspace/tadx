package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/config"
)

func TestRunCapabilityListRendersTOON(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--domain", "capability"}, &stdout, app.Options{})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
	assertGolden(t, "testdata/capability-list.toon", stdout.String())
}

func TestRunCapabilityListRendersJSON(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--domain", "capability", "--json"}, &stdout, app.Options{})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
	var document any
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout.String())
	}
}

func TestRunJSONUsageErrorIsOneDocumentWithoutNotice(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--not-a-flag", "--json"}, &stdout, app.Options{})
	if exitCode == 0 {
		t.Fatalf("expected usage failure, output=%s", stdout.String())
	}
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	if err := decoder.Decode(&document); err != nil {
		t.Fatalf("usage output is not JSON: %v\n%s", err, stdout.String())
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("expected one JSON document, extra=%v err=%v", extra, err)
	}
	if _, ok := document["error"]; !ok {
		t.Fatalf("usage document lacks error: %#v", document)
	}
}

func TestRunJSONFlagLastValueAndAlias(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		json bool
	}{
		{name: "explicit false", args: []string{"capability", "list", "--json=false"}},
		{name: "alias", args: []string{"capability", "list", "--jsn"}, json: true},
		{name: "last value", args: []string{"capability", "list", "--json", "--json=false"}},
		{name: "after inline value", args: []string{"capability", "list", "--config=portable/config.yaml", "--json"}, json: true},
		{name: "value is not mode", args: []string{"capability", "list", "--domain", "--json=true"}},
		{name: "numeric true", args: []string{"capability", "list", "--json=1"}, json: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			if exit := app.Run(context.Background(), test.args, &stdout, app.Options{}); exit != 0 {
				t.Fatalf("exit=%d output=%s", exit, stdout.String())
			}
			isJSON := strings.HasPrefix(strings.TrimSpace(stdout.String()), "{")
			if isJSON != test.json {
				t.Fatalf("JSON=%v output=%s", isJSON, stdout.String())
			}
		})
	}
}

func TestRunUsesExplicitCLIConfigPath(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/3.29/auth/signin" {
			t.Fatalf("request path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configContents := fmt.Sprintf("version: 1\ndefault_environment: production\nenvironments:\n  production:\n    url: %s\n    site_content_url: marketing\n    auth:\n      type: pat\n      pat_name_env: PROD_PAT_NAME\n      pat_secret_env: PROD_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(configContents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")

	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"auth", "check", "--config", configPath}, &stdout, app.Options{
		ConfigPath: filepath.Join(t.TempDir(), "missing.yaml"), HTTPClient: server.Client(),
	})
	if exitCode != 0 || !strings.Contains(stdout.String(), "status: authenticated") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
}

func TestEnvironmentProfileAndAuthStatusThroughCLI(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	options := app.Options{ConfigPath: configPath}

	run := func(args ...string) string {
		t.Helper()
		var stdout bytes.Buffer
		if exit := app.Run(context.Background(), args, &stdout, options); exit != 0 {
			t.Fatalf("Run(%v) exit = %d, output = %s", args, exit, stdout.String())
		}
		return stdout.String()
	}

	added := run("env", "add", "dev", "--url", "https://tableau.example.com", "--site", "test-site")
	if !strings.Contains(added, "status: added") || strings.Contains(added, "PAT_SECRET") {
		t.Fatalf("add output = %s", added)
	}
	run("env", "default", "dev")
	listed := run("env", "list")
	if !strings.Contains(listed, "environments[1]{alias,default}") || !strings.Contains(listed, "dev,true") {
		t.Fatalf("list output = %s", listed)
	}
	t.Setenv("TADX_DEV_PAT_NAME", "pat-name")
	t.Setenv("TADX_DEV_PAT_SECRET", "pat-secret")
	status := run("auth", "status")
	if !strings.Contains(status, "status: ready") || !strings.Contains(status, "environment: dev") || strings.Contains(status, "pat-name") || strings.Contains(status, "pat-secret") {
		t.Fatalf("auth status output = %s", status)
	}
}

func TestNamedWorkspaceCreateListAndStatusThroughCLI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace-root")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	options := app.Options{ConfigPath: configPath, UserHomeDir: func() (string, error) {
		return "", errors.New("explicit path must not resolve the user home directory")
	}}
	run := func(args ...string) string {
		t.Helper()
		var stdout bytes.Buffer
		if exit := app.Run(context.Background(), args, &stdout, options); exit != 0 {
			t.Fatalf("Run(%v) exit = %d, output = %s", args, exit, stdout.String())
		}
		return stdout.String()
	}

	created := run("workspace", "create", "development", "--path", root)
	if !strings.Contains(created, "status: created") || strings.Contains(created, root) {
		t.Fatalf("create output = %s", created)
	}
	listed := run("workspace", "list")
	if !strings.Contains(listed, "development,true,true") {
		t.Fatalf("list output = %s", listed)
	}
	status := run("workspace", "status", "--workspace", "development")
	if !strings.Contains(status, "status: ready") || !strings.Contains(status, "name: development") || strings.Contains(status, root) {
		t.Fatalf("status output = %s", status)
	}
}

func TestNamedWorkspaceCreateUsesDefaultHumanAccessibleRoot(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	options := app.Options{ConfigPath: configPath, UserHomeDir: func() (string, error) { return home, nil }}
	var stdout bytes.Buffer
	if exit := app.Run(context.Background(), []string{"workspace", "create", "development"}, &stdout, options); exit != 0 {
		t.Fatalf("exit = %d, output = %s", exit, stdout.String())
	}
	want := filepath.Join(home, "TADX", "workspaces", "development")
	if _, err := os.Stat(filepath.Join(want, "tadx.yaml")); err != nil {
		t.Fatalf("default workspace manifest: %v", err)
	}
	configuration, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	assertSameExistingPath(t, configuration.Workspaces["development"].Path, want)
	stdout.Reset()
	if exit := app.Run(context.Background(), []string{"workspace", "create", "development"}, &stdout, options); exit == 0 {
		t.Fatalf("duplicate workspace name succeeded, output = %s", stdout.String())
	}
}

func TestNamedWorkspaceCloneUsesDefaultHumanAccessibleRoot(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	options := app.Options{ConfigPath: configPath, UserHomeDir: func() (string, error) { return home, nil }}
	run := func(args ...string) {
		t.Helper()
		var stdout bytes.Buffer
		if exit := app.Run(context.Background(), args, &stdout, options); exit != 0 {
			t.Fatalf("Run(%v) exit = %d, output = %s", args, exit, stdout.String())
		}
	}
	run("workspace", "create", "development")
	run("workspace", "clone", "development", "--name", "experiment")
	want := filepath.Join(home, "TADX", "workspaces", "experiment")
	if _, err := os.Stat(filepath.Join(want, "tadx.yaml")); err != nil {
		t.Fatalf("default clone manifest: %v", err)
	}
	configuration, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	assertSameExistingPath(t, configuration.Workspaces["experiment"].Path, want)
}

func TestNamedWorkspaceCreateRejectsEscapingDefaultName(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	options := app.Options{ConfigPath: configPath, UserHomeDir: func() (string, error) { return home, nil }}
	for _, name := range []string{"..", "nested/workspace", `nested\workspace`} {
		var stdout bytes.Buffer
		if exit := app.Run(context.Background(), []string{"workspace", "create", name}, &stdout, options); exit == 0 {
			t.Fatalf("name %q succeeded, output = %s", name, stdout.String())
		}
	}
	if _, err := os.Stat(filepath.Join(home, "TADX")); !os.IsNotExist(err) {
		t.Fatalf("default root created for invalid name: %v", err)
	}
}

func TestRunPreservesCapabilityContextForSetupFailures(t *testing.T) {
	t.Run("cache configuration", func(t *testing.T) {
		var stdout bytes.Buffer
		exit := app.Run(context.Background(), []string{"search", "--cache", "--type", "workbook", "--environment", "production"}, &stdout, app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")})
		if exit == 0 || !strings.Contains(stdout.String(), "operation: search") || !strings.Contains(stdout.String(), "environment: production") {
			t.Fatalf("exit = %d, output = %s", exit, stdout.String())
		}
	})

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", "signin-request")
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(writer, `{"error":{"code":"401001","summary":"Sign-in failed","detail":"Invalid credentials"}}`)
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configContents := fmt.Sprintf("version: 1\ndefault_environment: production\nenvironments:\n  production:\n    url: %s\n    site_content_url: marketing\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PROD_PAT_NAME\n      pat_secret_env: PROD_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(configContents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	options := app.Options{ConfigPath: configPath, HTTPClient: server.Client(), MutationsEnabled: true}
	workspace := createNamedWorkspace(t, configPath, "authentication")
	pulled, err := artifact.NewWorkbookManager(nil).Pull(context.Background(), artifact.WorkbookPull{Workspace: workspace, Filename: "Finance.twb", Content: []byte("<workbook/>"), Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceServerOrigin: server.URL, SourceSiteLUID: "site-1", SourceEnvironment: "production", SourceSite: "marketing", SourceProjectID: "project-1", SourceProjectName: "Ops"}})
	if err != nil {
		t.Fatal(err)
	}
	artifactSelector, err := filepath.Rel(workspace, pulled.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		operation string
		args      []string
	}{
		{name: "pull authentication", operation: "workbook.pull", args: []string{"content", "workbook", "pull", "--workspace", "authentication", "--environment", "production", "--id", "wb-1"}},
		{name: "publish authentication", operation: "workbook.publish", args: []string{"content", "workbook", "publish", "--workspace", "authentication", "--environment", "production", "--artifact", filepath.ToSlash(artifactSelector), "--project-id", "project-1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			exit := app.Run(context.Background(), test.args, &stdout, options)
			output := stdout.String()
			if exit == 0 || !strings.Contains(output, "operation: "+test.operation) || !strings.Contains(output, "environment: production") || !strings.Contains(output, "site: marketing") || !strings.Contains(output, "tableau_request_id: signin-request") {
				t.Fatalf("exit = %d, output = %s", exit, output)
			}
		})
	}
}

func TestRunCapabilityGetRendersDetail(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "get", "capability.list"}, &stdout, app.Options{})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
	assertGolden(t, "testdata/capability-get.toon", stdout.String())
}

func TestCapabilityGetUsesListDomainAndResourceClassification(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "get", "workbook.list"}, &stdout, app.Options{})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
	for _, field := range []string{"domain: content", "resource: workbook"} {
		if !strings.Contains(stdout.String(), field) {
			t.Errorf("output missing %q: %s", field, stdout.String())
		}
	}
}

func TestCapabilityHelpDerivesFromRegistry(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--help"}, &stdout, app.Options{})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
	assertGolden(t, "testdata/capability-list-help.txt", stdout.String())
}

func TestRunUnknownCapabilityReturnsStructuredOperationError(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "get", "missing"}, &stdout, app.Options{})
	if exitCode != 1 || !strings.Contains(stdout.String(), "kind: operation") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
}

func TestRunUnknownFlagReturnsStructuredUsageError(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--unknown"}, &stdout, app.Options{})
	if exitCode != 2 || !strings.Contains(stdout.String(), "kind: usage") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
}

func TestMutationDiscoveryFilterReportsExecutionEnabled(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--domain", "content", "--resource", "workbook", "--mutation=true"}, &stdout, app.Options{MutationsEnabled: true})
	if exitCode != 0 || !strings.Contains(stdout.String(), "workbook.publish") || !strings.Contains(stdout.String(), "false,true") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
}

func TestMutationDiscoveryDoesNotRequireExecutionGate(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--domain", "content", "--resource", "workbook", "--mutation=true"}, &stdout, app.Options{})
	if exitCode != 0 || !strings.Contains(stdout.String(), "workbook.publish") || !strings.Contains(stdout.String(), "false,false") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
}

func TestDefaultDiscoveryIncludesMutations(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--domain", "content", "--resource", "workbook", "--limit", "100"}, &stdout, app.Options{})
	if exitCode != 0 || !strings.Contains(stdout.String(), "workbook.publish") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
}

func TestCapabilityGetReportsMutationExecutionState(t *testing.T) {
	for _, test := range []struct {
		enabled bool
		want    string
	}{{false, "execution_enabled: false"}, {true, "execution_enabled: true"}} {
		var stdout bytes.Buffer
		exitCode := app.Run(context.Background(), []string{"capability", "get", "workbook.publish"}, &stdout, app.Options{MutationsEnabled: test.enabled})
		if exitCode != 0 || !strings.Contains(stdout.String(), "remote_mutation: true") || !strings.Contains(stdout.String(), test.want) {
			t.Fatalf("enabled = %t, exit code = %d, output = %s", test.enabled, exitCode, stdout.String())
		}
	}
}

func TestRemoteMutationGatePrecedesRuntimeSetupAcrossDomains(t *testing.T) {
	tests := [][]string{
		{"content", "workbook", "delete", "--environment", "missing", "--id", "workbook-1"},
		{"content", "workbook", "delete", "--environment", "missing", "--id", "workbook-1", "--preview=false"},
		{"content", "datasource", "delete", "--environment", "missing", "--id", "datasource-1"},
		{"content", "flow", "delete", "--environment", "missing", "--id", "flow-1"},
		{"content", "project", "create", "--environment", "missing", "--name", "New project"},
		{"admin", "user", "create", "--environment", "missing", "--name", "test@example.com", "--site-role", "Viewer"},
		{"admin", "group", "create", "--environment", "missing", "--name", "Test group"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args[:3], " "), func(t *testing.T) {
			var stdout bytes.Buffer
			options := app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}
			exitCode := app.Run(context.Background(), args, &stdout, options)
			failure := decodeDiagnosticFailure(t, stdout.String())
			if exitCode != 1 || failure.ID != "mutation.disabled" || !strings.Contains(failure.CorrectiveAction, commandhint.Command("--config", options.ConfigPath, "mutation", "status")) {
				t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
			}
		})
	}
}

func assertGolden(t *testing.T, path, got string) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.TrimSuffix(string(want), "\n") && got != string(want) {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func assertSameExistingPath(t *testing.T, got, want string) {
	t.Helper()
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat path %q: %v", got, err)
	}
	wantInfo, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat expected path %q: %v", want, err)
	}
	if !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("path %q does not identify expected path %q", got, want)
	}
}
