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
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestAdminGroupCreatePreviewApplyThroughCLI(t *testing.T) {
	var listCalls, createCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", "request-1")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/groups":
			listCalls.Add(1)
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><groups/></tsResponse>`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/groups":
			createCalls.Add(1)
			body, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(body), `name="TADX Test Group"`) {
				t.Errorf("create body = %s", body)
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><group id="group-1" name="TADX Test Group" minimumSiteRole="Viewer" externalUserEnabled="true"/></tsResponse>`)
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf("version: 1\ndefault_environment: production\nenvironments:\n  production:\n    url: %s\n    site_content_url: marketing\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PROD_PAT_NAME\n      pat_secret_env: PROD_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")

	var stdout bytes.Buffer
	code := app.Run(context.Background(), []string{"admin", "group", "create", "--environment", "production", "--name", "TADX Test Group"}, &stdout, withSiteMutationConsent(t, app.Options{ConfigPath: configPath, HTTPClient: server.Client()}, true))
	if code != 0 || listCalls.Load() != 2 || createCalls.Load() != 1 || !strings.Contains(stdout.String(), "group_luid: group-1") || !strings.Contains(stdout.String(), "minimum_site_role: Viewer") || !strings.Contains(stdout.String(), "external_user_enabled: true") {
		t.Fatalf("code=%d lists=%d creates=%d output=%s", code, listCalls.Load(), createCalls.Load(), stdout.String())
	}
}

func TestAdminUserCreateRetainsProviderReturnedSettingsThroughCLI(t *testing.T) {
	var listCalls, createCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/users":
			listCalls.Add(1)
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><users/></tsResponse>`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/users":
			createCalls.Add(1)
			body, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(body), `name="alex"`) {
				t.Errorf("create body = %s", body)
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><user id="user-1" name="alex" siteRole="Viewer" authSetting="SAML" identityPoolName="pool-a" email="alex@example.com" language="en" locale="en_US"/></tsResponse>`)
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf("version: 1\ndefault_environment: production\nenvironments:\n  production:\n    url: %s\n    site_content_url: marketing\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PROD_USER_PAT_NAME\n      pat_secret_env: PROD_USER_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROD_USER_PAT_NAME", "pat-name")
	t.Setenv("PROD_USER_PAT_SECRET", "pat-secret")

	var stdout bytes.Buffer
	args := []string{"admin", "user", "create", "--environment", "production", "--name", "alex", "--site-role", "Viewer", "--auth-setting", "SAML", "--identity-pool", "pool-a", "--email", "alex@example.com", "--language", "en", "--locale", "en_US", "--full"}
	code := app.Run(context.Background(), args, &stdout, withSiteMutationConsent(t, app.Options{ConfigPath: configPath, HTTPClient: server.Client()}, true))
	if code != 0 || listCalls.Load() != 2 || createCalls.Load() != 1 || !strings.Contains(stdout.String(), "luid: user-1") || !strings.Contains(stdout.String(), "identity_pool_name: pool-a") || !strings.Contains(stdout.String(), "email: alex@example.com") || !strings.Contains(stdout.String(), "language: en") || !strings.Contains(stdout.String(), "locale: en_US") {
		t.Fatalf("code=%d lists=%d creates=%d output=%s", code, listCalls.Load(), createCalls.Load(), stdout.String())
	}
}
