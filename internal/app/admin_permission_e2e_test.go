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

func TestAdminPermissionMutationsThroughCLI(t *testing.T) {
	tests := []struct {
		name, operation, initial, source           string
		preview, full, disabled, drift, badSuccess bool
		wantCode, wantReads, wantWrites            int
		wantOutput                                 string
	}{
		{name: "create preview", operation: "create", preview: true, wantReads: 1, wantOutput: "mode: preview"},
		{name: "create", operation: "create", wantReads: 3, wantWrites: 1, wantOutput: "status: created"},
		{name: "create full", operation: "create", full: true, wantReads: 3, wantWrites: 1, wantOutput: "tableau_request_id: permission-write"},
		{name: "same mode", operation: "create", initial: "Allow", wantReads: 2, wantOutput: "status: unchanged"},
		{name: "conflicting mode", operation: "create", initial: "Deny", wantCode: 1, wantReads: 1, wantOutput: "admin.permission.create.rule_conflict"},
		{name: "delete preview", operation: "delete", initial: "Allow", preview: true, wantReads: 1, wantOutput: "mode: preview"},
		{name: "delete", operation: "delete", initial: "Allow", wantReads: 3, wantWrites: 1, wantOutput: "status: deleted"},
		{name: "delete absent", operation: "delete", wantReads: 2, wantOutput: "status: unchanged"},
		{name: "delete different mode", operation: "delete", initial: "Deny", wantCode: 1, wantReads: 1, wantOutput: "admin.permission.delete.mode_mismatch"},
		{name: "create inherited", operation: "create", source: "inherited", wantCode: 1, wantReads: 1, wantOutput: "admin.permission.create.inherited_or_unknown"},
		{name: "delete inherited", operation: "delete", initial: "Allow", source: "inherited", wantCode: 1, wantReads: 1, wantOutput: "admin.permission.delete.inherited_or_unknown"},
		{name: "create drift", operation: "create", drift: true, wantCode: 1, wantReads: 2, wantOutput: "admin.permission.create.target_changed"},
		{name: "delete drift", operation: "delete", initial: "Allow", drift: true, wantCode: 1, wantReads: 2, wantOutput: "admin.permission.delete.target_changed"},
		{name: "unknown create", operation: "create", badSuccess: true, wantCode: 1, wantReads: 2, wantWrites: 1, wantOutput: "admin.permission.create.outcome_unknown"},
		{name: "unknown delete", operation: "delete", initial: "Allow", badSuccess: true, wantCode: 1, wantReads: 2, wantWrites: 1, wantOutput: "admin.permission.delete.outcome_unknown"},
		{name: "disabled create", operation: "create", disabled: true, wantCode: 1, wantOutput: "mutation.disabled"},
		{name: "disabled delete", operation: "delete", disabled: true, wantCode: 1, wantOutput: "mutation.disabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reads, writes, allCalls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				allCalls.Add(1)
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/auth/signin":
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"credentials":{"token":"permission-session-secret","site":{"id":"site-1"},"user":{"id":"u1"}}}`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/u1":
					io.WriteString(w, `<tsResponse><user id="u1" name="test-user"/></tsResponse>`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/workbooks/w1/permissions":
					n := reads.Add(1)
					mode := tt.initial
					if writes.Load() > 0 {
						if tt.operation == "create" {
							mode = "Allow"
						} else {
							mode = ""
						}
					}
					if tt.drift && n > 1 {
						mode = "Deny"
					}
					parent := ""
					if tt.source == "inherited" {
						parent = `<parent type="Project" id="p1"/>`
					}
					rule := ""
					if mode != "" {
						rule = fmt.Sprintf(`<granteeCapabilities><user id="u1"/><capabilities><capability name="Read" mode="%s"/></capabilities></granteeCapabilities>`, mode)
					}
					fmt.Fprintf(w, `<tsResponse><permissions><workbook id="w1"/>%s%s</permissions></tsResponse>`, parent, rule)
				case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site-1/workbooks/w1/permissions":
					writes.Add(1)
					w.Header().Set("X-Tableau-Request-Id", "permission-write")
					if tt.badSuccess {
						io.WriteString(w, `<tsResponse/>`)
						return
					}
					io.WriteString(w, `<tsResponse><permissions><workbook id="w1"/><granteeCapabilities><user id="u1"/><capabilities><capability name="Read" mode="Allow"/></capabilities></granteeCapabilities></permissions></tsResponse>`)
				case r.Method == http.MethodDelete && r.URL.Path == "/api/3.29/sites/site-1/workbooks/w1/permissions/users/u1/Read/Allow":
					writes.Add(1)
					w.Header().Set("X-Tableau-Request-Id", "permission-write")
					if tt.badSuccess {
						w.WriteHeader(http.StatusOK)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			config := fmt.Sprintf("version: 1\nenvironments:\n  test:\n    url: %s\n    site_content_url: ''\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PERMISSION_PAT_NAME\n      pat_secret_env: PERMISSION_PAT_SECRET\n", server.URL)
			if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PERMISSION_PAT_NAME", "permission-pat")
			t.Setenv("PERMISSION_PAT_SECRET", "permission-pat-secret")
			args := []string{"admin", "permission", tt.operation, "--environment", "test", "--kind", "workbook", "--id", "w1", "--principal-type", "user", "--principal-id", "u1", "--capability", "Read", "--mode", "Allow"}
			if tt.preview {
				args = append(args, "--preview")
			}
			if tt.full {
				args = append(args, "--full")
			}
			var stdout bytes.Buffer
			code := app.Run(context.Background(), args, &stdout, withSiteMutationConsent(t, app.Options{ConfigPath: configPath, HTTPClient: server.Client()}, !tt.disabled && !tt.preview))
			if code != tt.wantCode || reads.Load() != int32(tt.wantReads) || writes.Load() != int32(tt.wantWrites) || !strings.Contains(stdout.String(), tt.wantOutput) {
				t.Fatalf("code=%d reads=%d writes=%d output=%s", code, reads.Load(), writes.Load(), stdout.String())
			}
			if tt.disabled && allCalls.Load() != 0 {
				t.Fatalf("disabled mutation made %d network calls", allCalls.Load())
			}
			if code == 0 && !tt.full && strings.Contains(stdout.String(), "tableau_request_id:") {
				t.Fatalf("compact output contains full-only request ID: %s", stdout.String())
			}
			for _, secret := range []string{"permission-session-secret", "permission-pat-secret"} {
				if strings.Contains(stdout.String(), secret) {
					t.Fatal("output exposed a secret")
				}
			}
		})
	}
}
