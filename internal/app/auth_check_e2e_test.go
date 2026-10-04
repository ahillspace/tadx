package app_test

import (
	"context"
	"github.com/ahillspace/tadx/internal/app"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthCheckReportsAuthenticatedIdentityThroughCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin" {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
			return
		}
		http.Error(writer, "unexpected request", http.StatusNotFound)
	}))
	defer server.Close()

	configPath := writeCLIConfig(t, server.URL)
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	options := app.Options{ConfigPath: configPath, HTTPClient: server.Client(), CorrelationID: func() string { return "auth-e2e" }}

	var stdout strings.Builder
	if exit := app.Run(context.Background(), []string{"auth", "check", "--environment", "production"}, &stdout, options); exit != 0 {
		t.Fatalf("auth check exit = %d, output = %s", exit, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{"status: authenticated", "environment: production", "site_luid: site-1", "user_luid: user-1", "help[1]:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("auth check output missing %q: %s", want, output)
		}
	}
}
