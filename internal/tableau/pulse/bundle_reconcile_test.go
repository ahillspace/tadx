package pulse_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/tableau"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

const bundleCreateDocument = `{"name":"Revenue","description":"Portable","specification":{"datasource":{"id":"datasource-1"},"basic_specification":{"measure":{"field":"Revenue","aggregation":"AGGREGATION_SUM"},"time_dimension":{"field":"Date"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"},{"string_value":"East"}],"include_null":true}]}},"extension_options":{"allowed_dimensions":["Region"],"allowed_granularities":["GRANULARITY_BY_DAY"]},"representation_options":{"type":"NUMBER_FORMAT_TYPE_NUMBER","decimal_places":2,"number_units":{"singular_noun":"sale"}},"insights_options":{"settings":[{"type":"INSIGHT_TYPE_TOP_DRIVERS"}]},"comparisons":{"comparisons":[{"index":"9007199254740993","compare_config":{"comparison":"TIME_COMPARISON_YEAR_AGO_PERIOD"}}]},"datasource_goals":[{"value":9007199254740993}],"related_links":[{"link_name":"Details","link_url":"https://example.test/details"}],"certification":{"is_certified":true}}`

func TestBundleDefinitionReadbackChecksEverySubmittedBusinessSection(t *testing.T) {
	for _, tc := range []struct{ name, old, replacement string }{
		{name: "identical"},
		{name: "aggregation", old: "AGGREGATION_SUM", replacement: "AGGREGATION_AVERAGE"},
		{name: "fixed null filter", old: `"include_null":true`, replacement: `"include_null":false`},
		{name: "representation", old: `"decimal_places":2`, replacement: `"decimal_places":3`},
		{name: "raw nested representation", old: `"singular_noun":"sale"`, replacement: `"singular_noun":"order"`},
		{name: "goal exact number", old: `"value":9007199254740993`, replacement: `"value":9007199254740992`},
		{name: "comparisons", old: "TIME_COMPARISON_YEAR_AGO_PERIOD", replacement: "TIME_COMPARISON_NONE"},
		{name: "insights", old: "INSIGHT_TYPE_TOP_DRIVERS", replacement: "INSIGHT_TYPE_METRIC_FORECAST"},
		{name: "adjustable dimensions", old: `"allowed_dimensions":["Region"]`, replacement: `"allowed_dimensions":[]`},
		{name: "links", old: "https://example.test/details", replacement: "https://example.test/changed"},
		{name: "certification", old: `"is_certified":true`, replacement: `"is_certified":false`},
		{name: "description", old: `"description":"Portable"`, replacement: `"description":"Changed"`},
		{name: "datasource", old: `"id":"datasource-1"`, replacement: `"id":"other"`},
		{name: "missing submitted goals", old: `,"datasource_goals":[{"value":9007199254740993}]`, replacement: ""},
		{name: "unexpected nondefault nested option", old: `"extension_options":{`, replacement: `"extension_options":{"offset_from_today":7,`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readback := strings.Replace(bundleCreateDocument, `"name":"Revenue","description":"Portable"`, `"metadata":{"id":"definition-1","name":"Revenue","description":"Portable","created_at":"server-time"}`, 1)
			if tc.old != "" {
				readback = strings.Replace(readback, tc.old, tc.replacement, 1)
			}
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/api/-/pulse/definitions/definition-1" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				_, _ = io.WriteString(w, readback)
			}))
			defer server.Close()
			client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			err := client.VerifyBundleDefinition(context.Background(), "definition-1", "datasource-1", "site-1", json.RawMessage(bundleCreateDocument))
			if (err != nil) != (tc.old != "") || requests != 1 {
				t.Fatalf("err=%v requests=%d", err, requests)
			}
		})
	}
}

func TestBundleDefinitionReadbackRetriesVisibilityOnly(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			reads := 0
			readback := strings.Replace(bundleCreateDocument, `"name":"Revenue","description":"Portable"`, `"metadata":{"id":"definition-1","name":"Revenue","description":"Portable"}`, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reads++
				if reads == 1 {
					w.WriteHeader(status)
					return
				}
				_, _ = io.WriteString(w, readback)
			}))
			defer server.Close()
			client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			client.SetPollPolicy(time.Millisecond, time.Second)
			err := client.VerifyBundleDefinition(context.Background(), "definition-1", "datasource-1", "site-1", json.RawMessage(bundleCreateDocument))
			if status == http.StatusNotFound && (err != nil || reads != 2) {
				t.Fatalf("readiness err=%v reads=%d", err, reads)
			}
			if status == http.StatusForbidden && (err == nil || reads != 1) {
				t.Fatalf("forbidden err=%v reads=%d", err, reads)
			}
		})
	}
}

func TestBundleDefinitionReadbackAllowsOnlyKnownDefaultsAndProviderMetadata(t *testing.T) {
	readback := strings.Replace(bundleCreateDocument, `"name":"Revenue","description":"Portable"`, `"metadata":{"id":"definition-1","name":"Revenue","description":"Portable"},"server_version":"v2"`, 1)
	for _, change := range [][2]string{
		{`"datasource":{"id":"datasource-1"}`, `"datasource":{"id":"datasource-1"},"is_running_total":false,"temporality":"TEMPORALITY_OVER_TIME"`},
		{`"extension_options":{`, `"extension_options":{"offset_from_today":0,"use_dynamic_offset":false,`},
		{`"representation_options":{`, `"representation_options":{"currency_code":"CURRENCY_CODE_USD",`},
		{`"type":"INSIGHT_TYPE_TOP_DRIVERS"`, `"type":"INSIGHT_TYPE_TOP_DRIVERS","disabled":false`},
		{`"certification":{`, `"certification":{"modified_by":"server-user","modified_at":"server-time",`},
		{`[{"string_value":"West"},{"string_value":"East"}]`, `[{"string_value":"East"},{"string_value":"West"}]`},
	} {
		readback = strings.Replace(readback, change[0], change[1], 1)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, readback) }))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	if err := client.VerifyBundleDefinition(context.Background(), "definition-1", "datasource-1", "site-1", json.RawMessage(bundleCreateDocument)); err != nil {
		t.Fatal(err)
	}
}

func TestBundleMetricReadbackNeverFetchesSharedDefinition(t *testing.T) {
	for _, tc := range []struct{ name, definition, datasource, site, number, status string }{
		{"ready", "definition-1", "datasource-1", "site-1", "9007199254740993", "verified"},
		{"definition ownership", "other", "datasource-1", "site-1", "9007199254740993", "ownership_mismatch"},
		{"datasource ownership", "definition-1", "other", "site-1", "9007199254740993", "ownership_mismatch"},
		{"site ownership", "definition-1", "datasource-1", "other", "9007199254740993", "ownership_mismatch"},
		{"exact number", "definition-1", "datasource-1", "site-1", "9007199254740992", "specification_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/-/pulse/metrics/metric-1" {
					t.Errorf("unexpected shared definition read: %s", r.URL.Path)
					w.WriteHeader(403)
					return
				}
				reads++
				if reads == 1 {
					w.WriteHeader(404)
					return
				}
				_, _ = fmt.Fprintf(w, `{"id":"metric-1","definition_id":%q,"site_id":%q,"specification":{"datasource":{"id":%q},"offset":%s,"filters":[]}}`, tc.definition, tc.site, tc.datasource, tc.number)
			}))
			defer server.Close()
			client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			client.SetPollPolicy(time.Millisecond, time.Second)
			result, err := client.ReconcileBundleMetric(context.Background(), tableaupulse.ExpectedMetric{MetricLUID: "metric-1", DefinitionLUID: "definition-1", DatasourceLUID: "datasource-1", SiteLUID: "site-1", Specification: map[string]any{"offset": json.Number("9007199254740993"), "filters": []any{}}})
			if err != nil || result.Status != tc.status || reads != 2 || result.Metric.LUID != "metric-1" {
				t.Fatalf("result=%#v err=%v reads=%d", result, err, reads)
			}
		})
	}
}
