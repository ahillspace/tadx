package app_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestDoctorInvalidProfileReportsCauseAndBlockedChecks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	data := "version: 1\ndefault_environment: selected\nenvironments:\n  selected:\n    url: https://example.test\n    auth:\n      type: pat\n  unused:\n    url: not-a-url\n    auth:\n      type: pat\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := app.Run(t.Context(), []string{"doctor", "--environment", "selected", "--json"}, &out, app.Options{ConfigPath: path})
	var result struct {
		Counts struct{ Pass, Fail, Blocked int }
		Checks []struct {
			ID, Status, Cause string
			ConfigPath        string `json:"config_path"`
			BlockedBy         string `json:"blocked_by"`
		}
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code != 1 || result.Counts.Pass != 1 || result.Counts.Fail != 1 || result.Counts.Blocked != 4 {
		t.Fatalf("code=%d result=%s", code, out.Bytes())
	}
	if !strings.Contains(result.Checks[0].Cause, "unused") || !strings.Contains(result.Checks[0].Cause, "URL") || result.Checks[0].ConfigPath != path {
		t.Fatalf("configuration cause lost: %s", out.Bytes())
	}
	for _, check := range result.Checks[1:5] {
		if check.Status != "blocked" || check.BlockedBy != "config.valid" {
			t.Fatalf("dependent check incorrectly ran: %s", out.Bytes())
		}
	}
}
