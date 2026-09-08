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

func diagnosticOptions(t *testing.T, server *httptest.Server) app.Options {
	t.Helper()
	config := filepath.Join(t.TempDir(), "config.yaml")
	data := fmt.Sprintf("version: 1\nenvironments:\n  test:\n    url: %s\n    site_content_url: ''\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: DIAGNOSTIC_PAT_NAME\n      pat_secret_env: DIAGNOSTIC_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(config, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DIAGNOSTIC_PAT_NAME", "diagnostic-pat")
	t.Setenv("DIAGNOSTIC_PAT_SECRET", "diagnostic-pat-secret")
	return app.Options{ConfigPath: config, HTTPClient: server.Client(), MutationsEnabled: true}
}

func diagnosticSignIn(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/3.29/auth/signin" {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"credentials":{"token":"diagnostic-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
	return true
}

func TestAdminUserInputDiagnosticsThroughCLI(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"invalid setting", []string{"create", "--name", "test@example.com", "--site-role", "Unlicensed", "--auth-setting", "IntentionallyInvalidAuthSetting"}, "Supported --auth-setting values: ServerDefault, SAML, OpenID, TableauIDWithMFA"},
		{"role as setting", []string{"create", "--name", "test@example.com", "--site-role", "Creator", "--auth-setting", "Creator"}, "Supported --auth-setting values:"},
		{"missing role", []string{"create", "--name", "test@example.com", "--auth-setting", "ServerDefault"}, "--site-role"},
		{"invalid update", []string{"update", "--id", "user-1", "--auth-setting", "Creator"}, "Supported --auth-setting values:"},
		{"server default", []string{"create", "--name", "test@example.com", "--site-role", "Unlicensed", "--auth-setting", "ServerDefault"}, ""},
		{"SAML", []string{"create", "--name", "test@example.com", "--site-role", "Unlicensed", "--auth-setting", "SAML"}, ""},
		{"OpenID", []string{"create", "--name", "test@example.com", "--site-role", "Unlicensed", "--auth-setting", "OpenID"}, ""},
		{"MFA", []string{"create", "--name", "test@example.com", "--site-role", "Unlicensed", "--auth-setting", "TableauIDWithMFA"}, ""},
		{"IdP configuration", []string{"create", "--name", "test@example.com", "--site-role", "Unlicensed", "--idp-configuration-id", "idp-1"}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var reads, writes int
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				if r.Method != http.MethodGet {
					writes++
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				reads++
				_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><users/></tsResponse>`)
			}))
			defer server.Close()
			args := append([]string{"admin", "user"}, tt.args...)
			args = append(args, "--environment", "test", "--preview")
			var out bytes.Buffer
			code := app.Run(context.Background(), args, &out, diagnosticOptions(t, server))
			if writes != 0 || (tt.want == "" && code != 0) || (tt.want != "" && (code != 2 || !strings.Contains(out.String(), tt.want) || reads != 0)) {
				t.Fatalf("code=%d reads=%d writes=%d output=%s", code, reads, writes, out.String())
			}
			if strings.Contains(out.String(), "explicit environment, site,") {
				t.Fatalf("diagnostic requests unsupported site input: %s", out.String())
			}
		})
	}
}

func TestPermissionCapabilityDiagnosticsThroughCLI(t *testing.T) {
	for _, kind := range []string{"project", "datasource", "flow", "workbook"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !diagnosticSignIn(w, r) {
					t.Errorf("invalid capability reached resource API: %s %s", r.Method, r.URL.Path)
				}
			}))
			defer server.Close()
			var out bytes.Buffer
			args := []string{"admin", "permission", "create", "--environment", "test", "--kind", kind, "--id", "resource-1", "--principal-type", "group", "--principal-id", "group-1", "--capability", "View", "--mode", "Allow", "--preview"}
			code := app.Run(context.Background(), args, &out, diagnosticOptions(t, server))
			want := "Supported " + kind + " capabilities:"
			if code == 0 || !strings.Contains(out.String(), want) || !strings.Contains(out.String(), "Read") {
				t.Fatalf("code=%d output=%s", code, out.String())
			}
		})
	}
	var out bytes.Buffer
	if code := app.Run(context.Background(), []string{"admin", "permission", "create", "--help"}, &out, app.Options{}); code != 0 || !strings.Contains(out.String(), "project: ProjectLeader, Read, Write") || !strings.Contains(out.String(), "PulseMetricDefine") {
		t.Fatalf("permission help: code=%d output=%s", code, out.String())
	}
}

func TestMissingResourceDiagnosticFocusesOnIdentityThroughCLI(t *testing.T) {
	for _, tt := range []struct {
		name, code string
		args       []string
	}{
		{"user", "404002", []string{"admin", "user", "inspect"}},
		{"workbook", "404006", []string{"content", "workbook", "inspect"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `<tsResponse><error code="%s"><summary>Resource not found</summary><detail>The resource does not exist.</detail></error></tsResponse>`, tt.code)
			}))
			defer server.Close()
			var out bytes.Buffer
			args := append(tt.args, "--environment", "test", "--id", "missing-resource")
			code := app.Run(context.Background(), args, &out, diagnosticOptions(t, server))
			if code == 0 || !strings.Contains(out.String(), "exact resource LUID") || strings.Contains(out.String(), "Verify the server URL") || !strings.Contains(out.String(), "The resource does not exist.") {
				t.Fatalf("code=%d output=%s", code, out.String())
			}
		})
	}
}
