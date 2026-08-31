package app_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestRunCapabilityListRendersTOON(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--domain", "capability"}, &stdout, app.Options{})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
	assertGolden(t, "testdata/capability-list.toon", stdout.String())
}

func TestRunPreservesCapabilityContextForSetupFailures(t *testing.T) {
	t.Run("catalog configuration", func(t *testing.T) {
		var stdout bytes.Buffer
		exit := app.Run(context.Background(), []string{"catalog", "search", "--environment", "production"}, &stdout, app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")})
		if exit == 0 || !strings.Contains(stdout.String(), "operation: catalog.search") || !strings.Contains(stdout.String(), "environment: production") {
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
	for _, test := range []struct {
		name      string
		operation string
		args      []string
	}{
		{name: "pull authentication", operation: "workbook.pull", args: []string{"content", "workbook", "pull", "--environment", "production", "--id", "wb-1"}},
		{name: "publish authentication", operation: "workbook.publish", args: []string{"content", "workbook", "publish", "--environment", "production", "--artifact", "artifact", "--project-id", "project-1"}},
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

func TestMutationDiscoveryGate(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--domain", "content", "--resource", "workbook", "--mutation=true"}, &stdout, app.Options{MutationsEnabled: true})
	if exitCode != 0 || !strings.Contains(stdout.String(), "workbook.publish") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
}

func TestMutationDiscoveryRequiresEnvironmentGate(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--domain", "content", "--resource", "workbook", "--mutation=true"}, &stdout, app.Options{})
	if exitCode != 2 || !strings.Contains(stdout.String(), "kind: usage") || strings.Contains(stdout.String(), "workbook.publish") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
}

func TestDefaultDiscoveryHidesMutations(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--domain", "content", "--resource", "workbook", "--limit", "100"}, &stdout, app.Options{})
	if exitCode != 0 || strings.Contains(stdout.String(), "workbook.publish") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
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
