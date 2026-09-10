package app

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
)

func TestPulseFollowValidatesExactMetricAndTypedFollowerBeforePreviewOrWrite(t *testing.T) {
	var authCalls, metricGets, userGets, writes int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/auth/signin":
			authCalls++
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"admin-1"}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/-/pulse/metrics/missing-metric":
			metricGets++
			http.Error(w, `{"error":{"summary":"missing metric"}}`, http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/api/-/pulse/metrics/metric-1":
			metricGets++
			_, _ = io.WriteString(w, `{"metadata":{"id":"metric-1","name":"Sales"},"definition_id":"definition-1","specification":{}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/group-1":
			userGets++
			http.Error(w, `<tsResponse><error code="404"><summary>Not found</summary></error></tsResponse>`, http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/user-1":
			userGets++
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="analyst@example.test" siteRole="Explorer"/></tsResponse>`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/-/pulse/subscriptions:batchCreate":
			writes++
			_, _ = io.WriteString(w, `{"subscription_id":"subscription-1"}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "tadx.yaml")
	configuration := fmt.Sprintf(`version: 1
default_environment: production
environments:
  production:
    url: %s
    site_content_url: marketing
    api_version: "3.29"
    auth:
      type: pat
      pat_name_env: PULSE_FOLLOW_PAT_NAME
      pat_secret_env: PULSE_FOLLOW_PAT_SECRET
`, server.URL)
	if err := os.WriteFile(configPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PULSE_FOLLOW_PAT_NAME", "pat-name")
	t.Setenv("PULSE_FOLLOW_PAT_SECRET", "pat-secret")
	options := Options{ConfigPath: configPath, HTTPClient: server.Client(), MutationsEnabled: true}

	run := func(args ...string) (int, string) {
		var output bytes.Buffer
		code := Run(context.Background(), args, &output, options)
		return code, output.String()
	}

	if code, output := run("pulse", "metric", "follow", "--environment", "production", "--id", "metric-1", "--preview"); code == 0 || !strings.Contains(output, "exactly one of --user-id or --group-id") || authCalls != 0 {
		t.Fatalf("malformed input code=%d output=%q auth calls=%d", code, output, authCalls)
	}
	if code, output := run("pulse", "metric", "follow", "--environment", "production", "--id", "missing-metric", "--user-id", "user-1", "--preview"); code == 0 || !strings.Contains(output, "pulse.metric.follow.metric.resolve") || writes != 0 {
		t.Fatalf("missing metric code=%d output=%q writes=%d", code, output, writes)
	}
	if code, output := run("pulse", "metric", "follow", "--environment", "production", "--id", "metric-1", "--user-id", "group-1", "--preview"); code == 0 || !strings.Contains(output, "pulse.metric.follow.user.resolve") || writes != 0 {
		t.Fatalf("group as user code=%d output=%q writes=%d", code, output, writes)
	}
	if code, output := run("pulse", "metric", "follow", "--environment", "production", "--id", "metric-1", "--user-id", "user-1", "--preview"); code != 0 || !strings.Contains(output, "mode: preview") || writes != 0 {
		t.Fatalf("preview code=%d output=%q writes=%d", code, output, writes)
	}
	if code, output := run("pulse", "metric", "follow", "--environment", "production", "--id", "metric-1", "--user-id", "user-1"); code != 0 || !strings.Contains(output, "status: followed") || writes != 1 {
		t.Fatalf("execute code=%d output=%q writes=%d", code, output, writes)
	}
	if metricGets != 4 || userGets != 3 {
		t.Fatalf("metric gets=%d user gets=%d", metricGets, userGets)
	}
}
