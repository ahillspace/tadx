package app

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	definitioncreate "github.com/ahillspace/tadx/actions/pulse/definition/create"
	definitionget "github.com/ahillspace/tadx/actions/pulse/definition/get"
	definitionlist "github.com/ahillspace/tadx/actions/pulse/definition/list"
	metricfollowers "github.com/ahillspace/tadx/actions/pulse/metric/followers"
	metricget "github.com/ahillspace/tadx/actions/pulse/metric/get"
	metriclist "github.com/ahillspace/tadx/actions/pulse/metric/list"
	"github.com/ahillspace/tadx/internal/catalog"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

func TestPulseCatalogReadsUseNoAuthenticationOrNetwork(t *testing.T) {
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
	entries := []catalog.ResourceEntry{
		pulseCatalogEntry(t, now, pulseDefinitionKind, definition.LUID, definition.Name, "", "", definition),
		pulseCatalogEntry(t, now, pulseMetricKind, metric.LUID, metric.Name, definition.LUID, "", metric),
		pulseCatalogEntry(t, now, pulseFollowerKind, follower.LUID, follower.FollowerName, metric.LUID, follower.FollowerLUID, follower),
	}
	if err := catalog.NewStore(root, func() time.Time { return now }).UpsertResources(context.Background(), entries); err != nil {
		t.Fatal(err)
	}
	transport := &failNetworkTransport{}
	runtime, err := newRuntime(Options{ConfigPath: configPath, HTTPClient: &http.Client{Transport: transport}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	commands := newPulseCommands(runtime)

	definitions, err := commands.ListPulseDefinitions(context.Background(), definitionlist.Input{Catalog: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	gotDefinition, err := commands.GetPulseDefinition(context.Background(), definitionget.Input{Catalog: true, LUID: definition.LUID})
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := commands.ListPulseMetrics(context.Background(), metriclist.Input{Catalog: true, DefinitionLUID: definition.LUID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	gotMetric, err := commands.GetPulseMetric(context.Background(), metricget.Input{Catalog: true, LUID: metric.LUID})
	if err != nil {
		t.Fatal(err)
	}
	followers, err := commands.ListPulseMetricFollowers(context.Background(), metricfollowers.Input{Catalog: true, MetricLUID: metric.LUID})
	if err != nil {
		t.Fatal(err)
	}

	if transport.calls != 0 {
		t.Fatalf("network calls = %d", transport.calls)
	}
	if len(definitions.Definitions) != 1 || definitions.Source == nil || definitions.Source.Mode != "catalog" {
		t.Fatalf("definitions = %#v", definitions)
	}
	if gotDefinition.Definition.LUID != definition.LUID || gotDefinition.Source == nil || gotDefinition.Source.Mode != "catalog" || gotDefinition.RequestID != "" {
		t.Fatalf("definition = %#v", gotDefinition)
	}
	if len(metrics.Metrics) != 1 || metrics.Source == nil || metrics.Source.Mode != "catalog" {
		t.Fatalf("metrics = %#v", metrics)
	}
	if gotMetric.Metric.LUID != metric.LUID || gotMetric.Source == nil || gotMetric.Source.Mode != "catalog" || gotMetric.RequestID != "" {
		t.Fatalf("metric = %#v", gotMetric)
	}
	if len(followers.Subscriptions) != 1 || followers.Source == nil || followers.Source.Mode != "catalog" || followers.RequestID != "" {
		t.Fatalf("followers = %#v", followers)
	}
}

func TestPulseDefinitionFieldValidatorUsesExactRawIDsAndAggregationRules(t *testing.T) {
	schema := fieldcatalog.Schema{
		DatasourceLUID: "datasource-1",
		DatasourceName: "Orders",
		Fields: []fieldcatalog.Field{
			{ID: "[Revenue]", Caption: "Revenue", Role: "measure", DataType: "NUMBER"},
			{ID: "[Margin Ratio]", Caption: "Margin Ratio", Role: "measure", DataType: "NUMBER", RequiresUserAggregation: true},
			{ID: "[Order Date]", Caption: "Order Date", Role: "date", DataType: "DATE"},
			{ID: "[Region]", Caption: "Region", Role: "dimension", DataType: "STRING"},
			{ID: "[Hidden]", Caption: "Hidden", Role: "excluded", DataType: "STRING", Excluded: true, ExclusionReason: "internal"},
		},
	}
	adapter := resourcedatasource.NewSchemaAdapter(pulseSchemaIdentityStub{}, pulseSchemaStub{schema: schema})
	validator := &pulseDefinitionFieldValidator{schema: adapter}

	valid := definitioncreate.FieldReferences{DatasourceLUID: schema.DatasourceLUID, MeasureField: "[Revenue]", Aggregation: "AGGREGATION_SUM", TimeDimension: "[Order Date]", AllowedDimensions: []string{"[Region]"}}
	if err := validator.ValidateDefinitionFields(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	for name, input := range map[string]definitioncreate.FieldReferences{
		"caption instead of raw ID":   {DatasourceLUID: schema.DatasourceLUID, MeasureField: "Revenue", Aggregation: "AGGREGATION_SUM", TimeDimension: "[Order Date]"},
		"wrong date role":             {DatasourceLUID: schema.DatasourceLUID, MeasureField: "[Revenue]", Aggregation: "AGGREGATION_SUM", TimeDimension: "[Region]"},
		"excluded dimension":          {DatasourceLUID: schema.DatasourceLUID, MeasureField: "[Revenue]", Aggregation: "AGGREGATION_SUM", TimeDimension: "[Order Date]", AllowedDimensions: []string{"[Hidden]"}},
		"missing user aggregation":    {DatasourceLUID: schema.DatasourceLUID, MeasureField: "[Margin Ratio]", Aggregation: "AGGREGATION_SUM", TimeDimension: "[Order Date]"},
		"unexpected user aggregation": {DatasourceLUID: schema.DatasourceLUID, MeasureField: "[Revenue]", Aggregation: "AGGREGATION_USER", TimeDimension: "[Order Date]"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validator.ValidateDefinitionFields(context.Background(), input); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
	valid.MeasureField = "[Margin Ratio]"
	valid.Aggregation = "AGGREGATION_USER"
	if err := validator.ValidateDefinitionFields(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
}

func pulseCatalogEntry(t *testing.T, observedAt time.Time, kind, luid, name, parent, owner string, payload any) catalog.ResourceEntry {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return catalog.ResourceEntry{Environment: "production", Site: "marketing", Kind: kind, LUID: luid, Name: name, ProjectPath: parent, Owner: owner, Payload: encoded, Coverage: "detail", ObservedAt: observedAt}
}

type pulseSchemaIdentityStub struct{}

func (pulseSchemaIdentityStub) Get(context.Context, string) (tableaudatasource.Datasource, error) {
	return tableaudatasource.Datasource{LUID: "datasource-1", Name: "Orders"}, nil
}

type pulseSchemaStub struct{ schema fieldcatalog.Schema }

func (s pulseSchemaStub) Read(context.Context, string, string) (fieldcatalog.Schema, error) {
	return s.schema, nil
}
