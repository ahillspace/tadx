package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestDoctorInvalidProfileReportsFindingWithoutBlockingHealthyChecks(t *testing.T) {
	t.Setenv("SELECTED_PAT_NAME", "fixture-name")
	t.Setenv("SELECTED_PAT_SECRET", "fixture-secret")
	requests := 0
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost || r.URL.Path != "/api/3.29/auth/signin" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
	}))
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "settings.yaml")
	data := fmt.Sprintf("version: 1\ndefault_environment: selected\nenvironments:\n  selected:\n    url: %s\n    auth:\n      type: pat\n      pat_name_env: SELECTED_PAT_NAME\n      pat_secret_env: SELECTED_PAT_SECRET\n  unused:\n    url: not-a-url\n    auth:\n      type: pat\n", server.URL)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := app.Run(t.Context(), []string{"doctor", "--environment", "selected", "--json"}, &out, app.Options{ConfigPath: path, HTTPClient: server.Client()})
	var result struct {
		Counts struct{ Pass, Fail, Blocked int }
		Checks []struct {
			ID, Status, Summary string
			BlockedBy           string `json:"blocked_by"`
		}
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code != 1 || result.Counts.Pass != 4 || result.Counts.Fail != 1 || result.Counts.Blocked != 0 || requests != 1 {
		t.Fatalf("code=%d result=%s", code, out.Bytes())
	}
	if result.Checks[0].ID != "config.valid" || result.Checks[0].Status != "pass" || result.Checks[1].ID != "config.environment.unused" || !strings.Contains(result.Checks[1].Summary, "url") {
		t.Fatalf("entry finding lost: %s", out.Bytes())
	}
	for _, check := range result.Checks[2:] {
		if check.BlockedBy != "" {
			t.Fatalf("unrelated invalid entry blocked a healthy dependency: %s", out.Bytes())
		}
	}
	for _, rejected := range []string{"not-a-url", "fixture-name", "fixture-secret", "fixture-session"} {
		if strings.Contains(out.String(), rejected) {
			t.Fatalf("doctor exposed a rejected or credential value: %s", out.Bytes())
		}
	}
}
