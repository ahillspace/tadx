package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
	"github.com/ahillspace/tadx/internal/cache"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

func TestPulseDeletesPreviewThenRevalidateExactLiveTarget(t *testing.T) {
	var definitionGets, definitionDeletes, metricGets, metricDeletes int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/-/pulse/definitions/definition-1":
			definitionGets++
			_, _ = io.WriteString(writer, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"},"basic_specification":{"measure":{"field":"[Revenue]","aggregation":"AGGREGATION_SUM"},"time_dimension":{"field":"[Order Date]"}}}}`)
		case request.Method == http.MethodDelete && request.URL.Path == "/api/-/pulse/definitions/definition-1":
			definitionDeletes++
			writer.Header().Set("X-Tableau-Request-Id", "definition-delete-request")
			writer.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && request.URL.Path == "/api/-/pulse/metrics/metric-1":
			metricGets++
			_, _ = io.WriteString(writer, `{"metadata":{"id":"metric-1","name":"Revenue this month"},"definition_id":"definition-1","is_default":false,"specification":{}}`)
		case request.Method == http.MethodDelete && request.URL.Path == "/api/-/pulse/metrics/metric-1":
			metricDeletes++
			writer.Header().Set("X-Tableau-Request-Id", "metric-delete-request")
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
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
      pat_name_env: PULSE_DELETE_PAT_NAME
      pat_secret_env: PULSE_DELETE_PAT_SECRET
`, server.URL)
	if err := os.WriteFile(configPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PULSE_DELETE_PAT_NAME", "pat-name")
	t.Setenv("PULSE_DELETE_PAT_SECRET", "pat-secret")
	runtime, err := newRuntime(Options{ConfigPath: configPath, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	commands := newPulseCommands(runtime)

	definitionPreview, err := commands.dependencies().DefinitionDeleter.DeletePulseDefinition(context.Background(), pulsedefinition.DeleteInput{Environment: "production", LUID: "definition-1", Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	if definitionPreview.Plan.Mode != "preview" || definitionPreview.Result != nil || definitionGets != 1 || definitionDeletes != 0 {
		t.Fatalf("definition preview = %#v, gets = %d, deletes = %d", definitionPreview, definitionGets, definitionDeletes)
	}
	definitionResult, err := commands.dependencies().DefinitionDeleter.DeletePulseDefinition(context.Background(), pulsedefinition.DeleteInput{Environment: "production", LUID: "definition-1"})
	if err != nil {
		t.Fatal(err)
	}
	if definitionResult.Result == nil || definitionResult.Result.DefinitionLUID != "definition-1" || definitionResult.Result.TableauRequestID != "definition-delete-request" || definitionGets != 3 || definitionDeletes != 1 {
		t.Fatalf("definition result = %#v, gets = %d, deletes = %d", definitionResult, definitionGets, definitionDeletes)
	}

	metricPreview, err := commands.dependencies().MetricDeleter.DeletePulseMetric(context.Background(), pulsemetric.DeleteInput{Environment: "production", LUID: "metric-1", Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	if metricPreview.Plan.Mode != "preview" || metricPreview.Result != nil || metricGets != 1 || metricDeletes != 0 {
		t.Fatalf("metric preview = %#v, gets = %d, deletes = %d", metricPreview, metricGets, metricDeletes)
	}
	metricResult, err := commands.dependencies().MetricDeleter.DeletePulseMetric(context.Background(), pulsemetric.DeleteInput{Environment: "production", LUID: "metric-1"})
	if err != nil {
		t.Fatal(err)
	}
	if metricResult.Result == nil || metricResult.Result.MetricLUID != "metric-1" || metricResult.Result.TableauRequestID != "metric-delete-request" || metricGets != 3 || metricDeletes != 1 {
		t.Fatalf("metric result = %#v, gets = %d, deletes = %d", metricResult, metricGets, metricDeletes)
	}
}

func TestPulseCacheReadsUseNoAuthenticationOrNetwork(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "tadx.yaml")
	configuration := `version: 1
default_environment: production
environments:
  production:
    url: https://tableau.invalid
    site_content_url: marketing
    api_version: "3.29"
    auth:
      type: pat
      pat_name_env: MISSING_PULSE_PAT_NAME
      pat_secret_env: MISSING_PULSE_PAT_SECRET
`
	if err := os.WriteFile(configPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	definition := tableaupulse.Definition{LUID: "definition-1", Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "[Revenue]", TimeDimension: "[Order Date]", Configuration: json.RawMessage(`{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`)}
	metric := tableaupulse.Metric{LUID: "metric-1", Name: "Revenue this month", DefinitionLUID: definition.LUID, SiteLUID: "site-1", Specification: map[string]any{"measurement_period": map[string]any{"range": "RANGE_CURRENT_PARTIAL"}}}
	follower := tableaupulse.Subscription{LUID: "subscription-1", MetricLUID: metric.LUID, FollowerType: "USER", FollowerLUID: "user-1", FollowerName: "User One"}
	entries := []cache.ResourceEntry{
		pulseCacheEntry(t, now, "definition", definition.LUID, definition.Name, "", "", definition),
		pulseCacheEntry(t, now, "metric", metric.LUID, metric.Name, definition.LUID, "", metric),
		pulseCacheEntry(t, now, "pulse_follower_snapshot", metric.LUID, metric.LUID, "", "", map[string]any{"version": 1, "metric_luid": metric.LUID, "subscriptions": []pulsemetric.Subscription{{LUID: follower.LUID, MetricLUID: metric.LUID, FollowerType: follower.FollowerType, FollowerLUID: follower.FollowerLUID, FollowerName: follower.FollowerName}}}),
	}
	if err := targetCacheFixture(t, configPath, func() time.Time { return now }).UpsertResources(context.Background(), entries); err != nil {
		t.Fatal(err)
	}
	transport := &failNetworkTransport{}
	runtime, err := newRuntime(Options{ConfigPath: configPath, HTTPClient: &http.Client{Transport: transport}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	commands := newPulseCommands(runtime)

	definitions, err := commands.dependencies().DefinitionLister.ListPulseDefinitions(context.Background(), pulsedefinition.ListInput{Cache: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	gotDefinition, err := commands.dependencies().DefinitionInspector.InspectPulseDefinition(context.Background(), pulsedefinition.InspectInput{Cache: true, LUID: definition.LUID})
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := commands.dependencies().MetricLister.ListPulseMetrics(context.Background(), pulsemetric.ListInput{Cache: true, DefinitionLUID: definition.LUID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	gotMetric, err := commands.dependencies().MetricInspector.InspectPulseMetric(context.Background(), pulsemetric.InspectInput{Cache: true, LUID: metric.LUID})
	if err != nil {
		t.Fatal(err)
	}
	followers, err := commands.dependencies().MetricFollowers.ListPulseMetricFollowers(context.Background(), pulsemetric.FollowersInput{Cache: true, MetricLUID: metric.LUID})
	if err != nil {
		t.Fatal(err)
	}

	if transport.calls != 0 {
		t.Fatalf("network calls = %d", transport.calls)
	}
	if len(definitions.Definitions) != 1 || definitions.Source == nil || definitions.Source.Mode != "cache" {
		t.Fatalf("definitions = %#v", definitions)
	}
	if gotDefinition.Definition.LUID != definition.LUID || gotDefinition.Source == nil || gotDefinition.Source.Mode != "cache" || gotDefinition.RequestID != "" {
		t.Fatalf("definition = %#v", gotDefinition)
	}
	if len(metrics.Metrics) != 1 || metrics.Source == nil || metrics.Source.Mode != "cache" {
		t.Fatalf("metrics = %#v", metrics)
	}
	if gotMetric.Metric.LUID != metric.LUID || gotMetric.Source == nil || gotMetric.Source.Mode != "cache" || gotMetric.RequestID != "" {
		t.Fatalf("metric = %#v", gotMetric)
	}
	if len(followers.Subscriptions) != 1 || followers.Source == nil || followers.Source.Mode != "cache" || followers.RequestID != "" {
		t.Fatalf("followers = %#v", followers)
	}
}

func pulseCacheEntry(t *testing.T, observedAt time.Time, kind, luid, name, parent, owner string, payload any) cache.ResourceEntry {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return cache.ResourceEntry{Environment: "production", Site: "marketing", Kind: kind, LUID: luid, Name: name, ProjectPath: parent, Owner: owner, Payload: encoded, Coverage: "detail", ObservedAt: observedAt}
}
