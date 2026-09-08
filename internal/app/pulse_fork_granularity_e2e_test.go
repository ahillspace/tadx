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
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestPulseForkGranularityThroughCLI(t *testing.T) {
	for _, test := range []struct {
		name      string
		args      []string
		inherited string
		allowed   []string
		changed   []string
		wantError string
	}{
		{name: "today below monthly minimum", args: []string{"--period", "TODAY"}, wantError: "GRANULARITY_BY_DAY"},
		{name: "yesterday below monthly minimum", args: []string{"--period", "YESTERDAY"}, wantError: "GRANULARITY_BY_DAY"},
		{name: "week below monthly minimum", args: []string{"--period", "THIS_WEEK"}, wantError: "GRANULARITY_BY_WEEK"},
		{name: "rolling days below monthly minimum", args: []string{"--period", "LAST_30_DAYS"}, wantError: "GRANULARITY_BY_DAY"},
		{name: "custom days below monthly minimum", args: []string{"--period", "CUSTOM_N_DAYS", "--days", "45"}, wantError: "GRANULARITY_BY_DAY"},
		{name: "allowed month", args: []string{"--period", "MONTH_TO_DATE"}},
		{name: "allowed year", args: []string{"--period", "LAST_YEAR"}},
		{name: "allowed rolling days", args: []string{"--period", "LAST_30_DAYS"}, allowed: []string{"GRANULARITY_BY_DAY"}},
		{name: "allowed inherited month", args: []string{"--filter", "Region=West"}, inherited: "GRANULARITY_BY_MONTH"},
		{name: "disallowed inherited day", args: []string{"--filter", "Region=West"}, inherited: "GRANULARITY_BY_DAY", wantError: "GRANULARITY_BY_DAY"},
		{name: "missing allowed granularities", args: []string{"--period", "MONTH_TO_DATE"}, allowed: []string{}, wantError: "allowed granularities"},
		{name: "changed definition", args: []string{"--period", "MONTH_TO_DATE"}, changed: []string{"GRANULARITY_BY_YEAR"}, wantError: "GRANULARITY_BY_MONTH"},
	} {
		t.Run(test.name, func(t *testing.T) {
			allowed := test.allowed
			if allowed == nil {
				allowed = []string{"GRANULARITY_BY_MONTH", "GRANULARITY_BY_QUARTER", "GRANULARITY_BY_YEAR"}
			}
			inherited := test.inherited
			if inherited == "" {
				inherited = "GRANULARITY_BY_MONTH"
			}
			var writes, definitionGets int
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/auth/signin":
					_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/-/pulse/metrics/metric-1":
					_, _ = fmt.Fprintf(w, `{"metadata":{"id":"metric-1"},"definition_id":"definition-1","specification":{"measurement_period":{"granularity":%q,"range":"RANGE_CURRENT_PARTIAL"},"filters":[]}}`, inherited)
				case r.Method == http.MethodGet && r.URL.Path == "/api/-/pulse/definitions/definition-1":
					definitionGets++
					current := allowed
					if test.changed != nil && definitionGets > 1 {
						current = test.changed
					}
					encoded, _ := json.Marshal(current)
					_, _ = fmt.Fprintf(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"ds-1"}},"extension_options":{"allowed_dimensions":["Region"],"allowed_granularities":%s}}`, encoded)
				default:
					writes++
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"message":"unexpected write"}`)
				}
			}))
			defer server.Close()
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			configuration := fmt.Sprintf("version: 1\nenvironments:\n  test:\n    url: %s\n    site_content_url: test\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PULSE_FORK_PAT_NAME\n      pat_secret_env: PULSE_FORK_PAT_SECRET\n", server.URL)
			if err := os.WriteFile(configPath, []byte(configuration), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PULSE_FORK_PAT_NAME", "test-pat")
			t.Setenv("PULSE_FORK_PAT_SECRET", "test-secret")
			args := append([]string{"pulse", "metric", "fork", "--environment", "test", "--id", "metric-1", "--full"}, test.args...)
			if test.changed == nil {
				args = append(args, "--preview")
			}
			options := app.Options{ConfigPath: configPath, HTTPClient: server.Client(), MutationsEnabled: true}
			var stdout bytes.Buffer
			code := app.Run(context.Background(), args, &stdout, options)
			if test.wantError == "" {
				if code != 0 || writes != 0 {
					t.Fatalf("code=%d writes=%d output=%s", code, writes, stdout.String())
				}
				return
			}
			if code == 0 || writes != 0 || !strings.Contains(stdout.String(), test.wantError) {
				t.Fatalf("code=%d writes=%d output=%s, want %q", code, writes, stdout.String(), test.wantError)
			}
			if test.changed == nil {
				stdout.Reset()
				code = app.Run(context.Background(), args[:len(args)-1], &stdout, options)
				if code == 0 || writes != 0 || !strings.Contains(stdout.String(), test.wantError) {
					t.Fatalf("create code=%d writes=%d output=%s, want %q", code, writes, stdout.String(), test.wantError)
				}
			}
		})
	}
}
