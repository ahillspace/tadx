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
	if exitCode != 0 || !strings.Contains(output, "7 checks completed") || !strings.Contains(output, "auth.tableau.connectivity") {
		t.Fatalf("exit code = %d, output = %s", exitCode, output)
	}
	for _, forbidden := range []string{"private-name", "private-secret", configPath, server.URL} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("doctor output exposed forbidden value %q: %s", forbidden, output)
		}
	}
}
