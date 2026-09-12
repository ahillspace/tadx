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
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/toon"
)

func TestDoctorRunsAllChecksWithoutExposingSecretsOrPaths(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/3.29/auth/signin" {
			t.Fatalf("request path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf("version: 1\ndefault_environment: production\nenvironments:\n  production:\n    url: %s\n    site_content_url: marketing\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PROD_PAT_NAME\n      pat_secret_env: PROD_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROD_PAT_NAME", "private-name")
	t.Setenv("PROD_PAT_SECRET", "private-secret")

	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"doctor", "--environment", "production", "--full"}, &stdout, app.Options{ConfigPath: configPath, HTTPClient: server.Client()})
	output := stdout.String()
	if exitCode != 0 || !strings.Contains(output, "6 checks completed") || !strings.Contains(output, "auth.tableau.connectivity") {
		t.Fatalf("exit code = %d, output = %s", exitCode, output)
	}
	if strings.Contains(strings.ToLower(output), "mcp") {
		t.Fatalf("doctor reported Tableau MCP state: %s", output)
	}
	assertNoDiagnosticValues(t, output, configPath, "private-name", "private-secret", "session-token", configPath, server.URL)
}

// Explicit configuration belongs in copyable recovery commands, but not in
// diagnostic details. Decode first so Windows escaping cannot hide a leak.
func assertNoDiagnosticValues(t *testing.T, output, configPath string, forbidden ...string) {
	t.Helper()
	decoded, err := toon.Decode([]byte(output))
	if err != nil {
		t.Fatal(err)
	}
	var check func(any)
	check = func(value any) {
		switch v := value.(type) {
		case string:
			text := strings.ReplaceAll(v, commandhint.Command("--config", configPath)+" ", "tadx ")
			for _, secret := range forbidden {
				if strings.Contains(text, secret) {
					t.Fatalf("diagnostic contains forbidden value %q outside an explicit config hint: %s", secret, v)
				}
			}
		case map[string]any:
			for _, item := range v {
				check(item)
			}
		case []any:
			for _, item := range v {
				check(item)
			}
		}
	}
	check(decoded)
}

func TestDoctorFailResultExitsOneWithoutRenderingASecondDocument(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"doctor"}, &stdout, app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")})
	output := stdout.String()
	if exitCode != 1 {
		t.Fatalf("exit code = %d, output = %s", exitCode, output)
	}
	if strings.Count(output, "counts:") != 1 || strings.Contains(output, "error:") {
		t.Fatalf("doctor rendered more than one diagnostic document: %s", output)
	}
}
