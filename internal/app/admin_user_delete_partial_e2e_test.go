package app_test

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestAdminUserDeleteRetainsUnlicensedPartialOutcomeThroughCLI(t *testing.T) {
	var gets, deletes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"admin-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/users/user-1":
			gets.Add(1)
			role := "Viewer"
			if gets.Load() == 3 {
				role = "Unlicensed"
			}
			_, _ = fmt.Fprintf(writer, `<tsResponse><user id="user-1" name="alex@example.com" siteRole="%s"/></tsResponse>`, role)
		case request.Method == http.MethodDelete && request.URL.Path == "/api/3.29/sites/site-1/users/user-1":
			deletes.Add(1)
			writer.Header().Set("X-Tableau-Request-Id", "delete-request")
			writer.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(writer, `<tsResponse><error code="409003"><summary>User asset conflict</summary><detail>The specified user still owns content and cannot be deleted.</detail></error></tsResponse>`)
		default:
			t.Errorf("unexpected Tableau request: %s %s", request.Method, request.URL.Path)
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf("version: 1\ndefault_environment: production\nenvironments:\n  production:\n    url: %s\n    site_content_url: team-site\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PROD_DELETE_PAT_NAME\n      pat_secret_env: PROD_DELETE_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROD_DELETE_PAT_NAME", "pat-name")
	t.Setenv("PROD_DELETE_PAT_SECRET", "pat-secret")

	var output bytes.Buffer
	code := app.Run(context.Background(), []string{"admin", "user", "delete", "--environment", "production", "--id", "user-1", "--json"}, &output, withSiteMutationConsent(t, app.Options{ConfigPath: configPath, HTTPClient: server.Client()}, true))
	if code == 0 || deletes.Load() != 1 || gets.Load() != 3 || !json.Valid(output.Bytes()) {
		t.Fatalf("code=%d gets=%d deletes=%d output=%s", code, gets.Load(), deletes.Load(), output.String())
	}
	var document struct {
		Output struct {
			Details string `json:"details"`
			Result  struct {
				Status        string `json:"status"`
				UserLUID      string `json:"user_luid"`
				RemovalStatus string `json:"removal_status"`
				LicenseStatus string `json:"license_status"`
			} `json:"result"`
			Help []string `json:"help"`
		} `json:"output"`
		Error struct {
			ID               string `json:"id"`
			UpstreamCode     string `json:"upstream_code"`
			Phase            string `json:"phase"`
			Outcome          string `json:"outcome"`
			CorrectiveAction string `json:"corrective_action"`
		} `json:"error"`
	}
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(document.Output.Details, "last --full --json") || !strings.Contains(document.Output.Details, "--config") {
		t.Fatalf("saved partial result lacks contextual expansion: %s", output.String())
	}
	result := document.Output.Result
	if result.Status != "unlicensed" || result.UserLUID != "user-1" || result.RemovalStatus != "refused" || result.LicenseStatus != "unlicensed" {
		t.Fatalf("result=%#v output=%s", result, output.String())
	}
	if document.Error.ID != "admin.user.delete.partial" || document.Error.UpstreamCode != "409003" || document.Error.Phase != "verification" || document.Error.Outcome != "confirmed" || !strings.Contains(document.Error.CorrectiveAction, "admin user inspect") || strings.Contains(document.Error.CorrectiveAction, "Run without --preview") {
		t.Fatalf("error=%#v output=%s", document.Error, output.String())
	}
}
