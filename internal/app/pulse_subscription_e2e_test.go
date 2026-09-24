package app

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

	"github.com/ahillspace/tadx/internal/managedpolicy"
)

func TestPulseSubscriptionListUsesAuthenticatedUserAndExactEnrichment(t *testing.T) {
	var paths []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.RequestURI())
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/-/pulse/subscriptions":
			if r.URL.Query().Get("user_id") != "user-1" || r.URL.Query().Get("metric_id") != "" {
				t.Errorf("unexpected subscription query: %s", r.URL.RawQuery)
			}
			if r.URL.Query().Get("page_token") == "" {
				_, _ = io.WriteString(w, `{"subscriptions":[{"id":"sub-1","metric_id":"metric-1","follower":{"user_id":"user-1"}}],"next_page_token":"next"}`)
			} else {
				_, _ = io.WriteString(w, `{"subscriptions":[{"id":"sub-2","metric_id":"metric-2","follower":{"user_id":"user-1"}}]}`)
			}
		case "/api/-/pulse/metrics:batchGet":
			if r.Method != http.MethodPost {
				t.Errorf("metric batch method: %s", r.Method)
			}
			var request struct {
				IDs []string `json:"metric_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.IDs) != 2 || request.IDs[0] != "metric-1" || request.IDs[1] != "metric-2" {
				t.Errorf("metric batch IDs: %+v err=%v", request.IDs, err)
			}
			_, _ = io.WriteString(w, `{"metrics":[{"id":"metric-1","definition_id":"definition-1","specification":{"filters":[{"field":"Region","values":["West"]}],"measurement_period":{"range":"LAST_30_DAYS"}}}]}`)
		case "/api/-/pulse/definitions:batchGet":
			if r.Method != http.MethodPost {
				t.Errorf("definition batch method: %s", r.Method)
			}
			if r.URL.Query().Get("view") != "DEFINITION_VIEW_BASIC" {
				t.Errorf("definition batch view: %s", r.URL.RawQuery)
			}
			var request struct {
				IDs []string `json:"definition_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.IDs) != 1 || request.IDs[0] != "definition-1" {
				t.Errorf("definition batch IDs: %+v err=%v", request.IDs, err)
			}
			_, _ = io.WriteString(w, `{"definitions":[{"metadata":{"id":"definition-1","name":"Revenue"}}]}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	var output bytes.Buffer
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	config := fmt.Sprintf("version: 1\ndefault_environment: test\nenvironments:\n  test:\n    url: %s\n    site_content_url: test\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: SUBSCRIPTION_PAT_NAME\n      pat_secret_env: SUBSCRIPTION_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUBSCRIPTION_PAT_NAME", "fixture-pat")
	t.Setenv("SUBSCRIPTION_PAT_SECRET", "fixture-secret")
	options := Options{ConfigPath: configPath, HTTPClient: server.Client(), managedPolicy: fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"pulse.subscription.list": true}}}
	code := Run(context.Background(), []string{"pulse", "subscription", "list", "--env", "test", "--limit", "100", "--json"}, &output, options)
	if code != 0 || !strings.Contains(output.String(), "sub-1") || !strings.Contains(output.String(), "sub-2") || !strings.Contains(output.String(), "Revenue") || !strings.Contains(output.String(), "partial") {
		t.Fatalf("code=%d output=%s", code, output.String())
	}
	for _, path := range paths {
		if strings.Contains(path, "/users") || strings.Contains(path, "/metrics/") || strings.Contains(path, "/definitions/") || strings.Contains(path, "metrics?") || !strings.HasPrefix(path, "GET ") && !strings.Contains(path, "batchGet") && !strings.Contains(path, "/auth/signin") {
			t.Errorf("unexpected broad or mutating request: %s", path)
		}
	}
}

func TestPulseSubscriptionHelpNeedsNoConfigurationOrAuthentication(t *testing.T) {
	for _, args := range [][]string{
		{"pulse", "subscription", "-h"},
		{"pulse", "subscription", "list", "--help"},
		{"help", "pulse", "subscription", "list"},
	} {
		var output bytes.Buffer
		code := Run(t.Context(), args, &output, Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml"), managedPolicy: fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{}}})
		if code != 0 || !strings.Contains(output.String(), "subscription list") || !strings.Contains(output.String(), "--limit") {
			t.Fatalf("args=%v code=%d output=%s", args, code, output.String())
		}
	}
}

func TestPulseSubscriptionInvalidLimitStopsBeforeAuthentication(t *testing.T) {
	for _, limit := range []string{"0", "-1", "10001"} {
		var output bytes.Buffer
		code := Run(t.Context(), []string{"pulse", "subscription", "list", "--limit", limit, "--json"}, &output, Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml"), managedPolicy: fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"pulse.subscription.list": true}}})
		if code == 0 || !strings.Contains(output.String(), "--limit must be between 1 and 10000") || strings.Contains(output.String(), "configuration") {
			t.Fatalf("limit=%s code=%d output=%s", limit, code, output.String())
		}
	}
}
