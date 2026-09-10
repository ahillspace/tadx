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

func TestAdminPermissionWrongTypedPrincipalReportsRecoveryWithoutWrites(t *testing.T) {
	for _, test := range []struct {
		name, principalType, principalID, endpoint, hint string
		status                                           int
		wantRetryable                                    string
	}{
		{name: "group missing", principalType: "group", principalID: "user-1", endpoint: "/api/3.29/sites/site-1/groups/user-1/users", hint: "admin group inspect --id user-1 --environment test", status: http.StatusNotFound, wantRetryable: "retryable: false"},
		{name: "user missing", principalType: "user", principalID: "group-1", endpoint: "/api/3.29/sites/site-1/users/group-1", hint: "admin user inspect --id group-1 --environment test", status: http.StatusNotFound, wantRetryable: "retryable: false"},
		{name: "group transient", principalType: "group", principalID: "group-1", endpoint: "/api/3.29/sites/site-1/groups/group-1/users", hint: "admin group inspect --id group-1 --environment test", status: http.StatusServiceUnavailable, wantRetryable: "retryable: true"},
	} {
		for _, operation := range []string{"create", "delete"} {
			t.Run(test.name+"/"+operation, func(t *testing.T) {
				var groupReads, permissionReads, writes int
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/auth/signin":
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"admin-1"}}}`)
					case r.Method == http.MethodGet && r.URL.Path == test.endpoint:
						groupReads++
						http.Error(w, `<tsResponse><error><summary>Unavailable</summary></error></tsResponse>`, test.status)
					case strings.Contains(r.URL.Path, "/permissions"):
						if r.Method == http.MethodGet {
							permissionReads++
						} else {
							writes++
						}
						w.WriteHeader(http.StatusInternalServerError)
					default:
						t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
					}
				}))
				defer server.Close()

				configPath := filepath.Join(t.TempDir(), "tadx.yaml")
				config := fmt.Sprintf(`version: 1
environments:
  test:
    url: %s
    site_content_url: marketing
    api_version: "3.29"
    auth:
      type: pat
      pat_name_env: PERMISSION_PRINCIPAL_PAT_NAME
      pat_secret_env: PERMISSION_PRINCIPAL_PAT_SECRET
`, server.URL)
				if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PERMISSION_PRINCIPAL_PAT_NAME", "pat-name")
				t.Setenv("PERMISSION_PRINCIPAL_PAT_SECRET", "pat-secret")
				args := []string{"admin", "permission", operation, "--environment", "test", "--kind", "workbook", "--id", "workbook-1", "--principal-type", test.principalType, "--principal-id", test.principalID, "--capability", "Read", "--mode", "Allow", "--preview"}
				var output bytes.Buffer
				code := app.Run(context.Background(), args, &output, app.Options{ConfigPath: configPath, HTTPClient: server.Client()})
				text := output.String()
				for _, want := range []string{"admin.permission." + operation + ".principal.resolve", "phase: verification", "outcome: not_attempted", "kind: " + test.principalType, "resource: " + test.principalID, test.hint, test.wantRetryable} {
					if !strings.Contains(text, want) {
						t.Fatalf("output missing %q: %s", want, text)
					}
				}
				if code == 0 || groupReads != 1 || permissionReads != 0 || writes != 0 {
					t.Fatalf("code=%d group reads=%d permission reads=%d writes=%d output=%s", code, groupReads, permissionReads, writes, text)
				}
			})
		}
	}
}
