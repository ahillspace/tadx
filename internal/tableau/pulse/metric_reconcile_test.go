package pulse_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/tableau"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

func TestExactReconciliationRetriesOnlyUnreadyObjectsAndFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		metric404, definition404 int
		mismatch                 string
		wantStatus               string
		wantError                bool
	}{
		{name: "ready", wantStatus: "verified"},
		{name: "metric not ready", metric404: 1, wantStatus: "verified"},
		{name: "definition not ready does not reread metric", definition404: 2, wantStatus: "verified"},
		{name: "metric never ready", metric404: 1000, wantStatus: "pending_readback", wantError: true},
		{name: "definition never ready", definition404: 1000, wantStatus: "pending_readback", wantError: true},
		{name: "wrong period", mismatch: "period", wantStatus: "specification_mismatch"},
		{name: "wrong filter", mismatch: "filter", wantStatus: "specification_mismatch"},
		{name: "wrong definition", mismatch: "definition", wantStatus: "ownership_mismatch"},
		{name: "wrong datasource", mismatch: "datasource", wantStatus: "ownership_mismatch"},
		{name: "contradictory metric datasource", mismatch: "metric datasource", wantStatus: "ownership_mismatch"},
		{name: "wrong site", mismatch: "site", wantStatus: "ownership_mismatch"},
		{name: "wrong metric identity", mismatch: "identity", wantStatus: "pending_readback", wantError: true},
		{name: "forbidden", mismatch: "forbidden", wantStatus: "pending_readback", wantError: true},
		{name: "canceled", mismatch: "cancel", wantStatus: "pending_readback", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			metricReads, definitionReads := 0, 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/-/pulse/metrics/metric-1":
					metricReads++
					if tc.mismatch == "cancel" {
						cancel()
						w.WriteHeader(404)
						return
					}
					if tc.mismatch == "forbidden" {
						w.WriteHeader(403)
						return
					}
					if metricReads <= tc.metric404 {
						w.WriteHeader(404)
						return
					}
					period, filter, definition, site, id := "RANGE_CURRENT_PARTIAL", "West", "definition-1", "site-1", "metric-1"
					switch tc.mismatch {
					case "period":
						period = "RANGE_LAST_COMPLETE"
					case "filter":
						filter = "East"
					case "definition":
						definition = "other"
					case "site":
						site = "other"
					case "identity":
						id = "other"
					}
					datasource := "datasource-1"
					if tc.mismatch == "metric datasource" {
						datasource = "other"
					}
					_, _ = fmt.Fprintf(w, `{"id":%q,"definition_id":%q,"site_id":%q,"specification":{"datasource":{"id":%q},"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":%q},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":%q}],"include_null":false}]}}`, id, definition, site, datasource, period, filter)
				case "/api/-/pulse/definitions/definition-1":
					definitionReads++
					if definitionReads <= tc.definition404 {
						w.WriteHeader(404)
						return
					}
					ds := "datasource-1"
					if tc.mismatch == "datasource" {
						ds = "other"
					}
					_, _ = fmt.Fprintf(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":%q}}}`, ds)
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					w.WriteHeader(403)
				}
			}))
			defer server.Close()
			client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			timeout := 2 * time.Second
			if tc.metric404 == 1000 || tc.definition404 == 1000 {
				timeout = 100 * time.Millisecond
			}
			client.SetPollPolicy(time.Millisecond, timeout)
			var spec map[string]any
			if err := json.Unmarshal([]byte(`{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"}],"include_null":false}]}`), &spec); err != nil {
				t.Fatal(err)
			}
			result, err := client.ReconcileMetric(ctx, tableaupulse.ExpectedMetric{MetricLUID: "metric-1", DefinitionLUID: "definition-1", DatasourceLUID: "datasource-1", SiteLUID: "site-1", Specification: spec})
			if (err != nil) != tc.wantError || result.Status != tc.wantStatus {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if tc.mismatch == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
			if tc.definition404 > 0 && metricReads != 1 {
				t.Fatalf("ready metric unnecessarily read %d times", metricReads)
			}
			if tc.wantStatus == "verified" && (metricReads != tc.metric404+1 || definitionReads != tc.definition404+1 || result.Metric.LUID != "metric-1" || result.Definition.LUID != "definition-1") {
				t.Fatalf("reads=%d/%d evidence=%#v", metricReads, definitionReads, result)
			}
		})
	}
}

func TestExactReconciliationPreservesNumberFidelityAndUnorderedFilterValues(t *testing.T) {
	spec := map[string]any{"offset": json.Number("9007199254740993"), "filters": []any{map[string]any{"field": "Region", "categorical_values": []any{map[string]any{"string_value": "West"}, map[string]any{"string_value": "East"}}}}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/-/pulse/metrics/metric-1" {
			_, _ = io.WriteString(w, `{"id":"metric-1","definition_id":"definition-1","specification":{"datasource":{"id":"datasource-1"},"offset":9007199254740993,"filters":[{"field":"Region","categorical_values":[{"string_value":"East"},{"string_value":"West"}]}]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`)
	}))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	result, err := client.ReconcileMetric(context.Background(), tableaupulse.ExpectedMetric{MetricLUID: "metric-1", DefinitionLUID: "definition-1", DatasourceLUID: "datasource-1", Specification: spec})
	if err != nil || result.Status != "verified" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestExactReconciliationComparesTypedFilterRepresentationsSemantically(t *testing.T) {
	tests := []struct {
		name          string
		savedFilter   string
		requestedSpec string
		wantStatus    string
	}{
		{
			name:          "text values equal categorical values",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"East"},{"string_value":"West"}],"include_null":false}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","values":["West","East"],"include_null":false}]}`,
			wantStatus:    "verified",
		},
		{
			name:          "conflicting dual representations",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"East"}],"values":["West"],"include_null":false}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"East"}],"include_null":false}]}`,
			wantStatus:    "specification_mismatch",
		},
		{
			name:          "native null category has no legacy text entry",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null},{"string_value":"West"}],"values":["West"],"include_null":true}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"},{"null_value":null}],"include_null":true}]}`,
			wantStatus:    "verified",
		},
		{
			name:          "typed null does not excuse conflicting string mirror",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null},{"string_value":"East"}],"values":["West"],"include_null":true}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null},{"string_value":"East"}],"include_null":true}]}`,
			wantStatus:    "specification_mismatch",
		},
		{
			name:          "unqualified boolean mirror remains strict",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"bool_value":true},{"string_value":"West"}],"values":["West"],"include_null":false}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"},{"bool_value":true}],"include_null":false}]}`,
			wantStatus:    "specification_mismatch",
		},
		{
			name:          "protobuf null enum equals native null",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null},{"string_value":"West"}],"values":["West"],"include_null":true}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":"NULL_VALUE"},{"string_value":"West"}],"include_null":true}]}`,
			wantStatus:    "verified",
		},
		{
			name:          "unknown typed attributes cannot be inferred from text mirror",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West","annotation":{"source":"owned"}}],"values":["West"]}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West","annotation":{"source":"owned"}}]}]}`,
			wantStatus:    "specification_mismatch",
		},
		{
			name:          "missing boolean population remains mismatch",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"}],"values":["West"]}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"},{"bool_value":true}]}]}`,
			wantStatus:    "specification_mismatch",
		},
		{
			name:          "changed unknown typed attributes remain mismatch",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West","annotation":{"source":"other"}}],"values":["West"]}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West","annotation":{"source":"owned"}}]}]}`,
			wantStatus:    "specification_mismatch",
		},
		{
			name:          "null member cannot be inferred from inclusion flag",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"}],"values":["West"],"include_null":true}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null},{"string_value":"West"}],"include_null":true}]}`,
			wantStatus:    "specification_mismatch",
		},
		{
			name:          "null metadata remains part of population",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null,"annotation":true}],"values":[],"include_null":true}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null}],"include_null":true}]}`,
			wantStatus:    "specification_mismatch",
		},
		{
			name:          "unknown categorical attributes match only when retained",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null,"annotation":{"source":"owned"}},{"string_value":"West"}],"include_null":true}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"},{"null_value":null,"annotation":{"source":"owned"}}],"include_null":true}]}`,
			wantStatus:    "verified",
		},
		{
			name:          "unknown null enum is not native null",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":"UNKNOWN_NULL"}],"values":[],"include_null":true}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null}],"include_null":true}]}`,
			wantStatus:    "specification_mismatch",
		},
		{
			name:          "null enum with unknown attributes remains distinct",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":"NULL_VALUE","annotation":true}],"include_null":true}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null,"annotation":true}],"include_null":true}]}`,
			wantStatus:    "specification_mismatch",
		},
		{
			name:          "native null without inclusion cannot omit text mirror",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null}],"values":[],"include_null":false}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null}],"include_null":false}]}`,
			wantStatus:    "specification_mismatch",
		},
		{
			name:          "duplicate typed population is not discarded",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"},{"string_value":"West"}],"values":["West","West"]}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"}]}]}`,
			wantStatus:    "specification_mismatch",
		},
		{
			name:          "saved null enum is normalized only for comparison",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":"NULL_VALUE"},{"string_value":"West"}],"values":["West"],"include_null":true}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null},{"string_value":"West"}],"include_null":true}]}`,
			wantStatus:    "verified",
		},
		{
			name:          "native null flag must remain true",
			savedFilter:   `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null},{"string_value":"West"}],"values":["West"],"include_null":false}`,
			requestedSpec: `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"null_value":null},{"string_value":"West"}],"include_null":true}]}`,
			wantStatus:    "specification_mismatch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/-/pulse/metrics/metric-1":
					_, _ = fmt.Fprintf(w, `{"id":"metric-1","definition_id":"definition-1","site_id":"site-1","specification":{"datasource":{"id":"datasource-1"},"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[%s]}}`, test.savedFilter)
				case "/api/-/pulse/definitions/definition-1":
					_, _ = io.WriteString(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`)
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					w.WriteHeader(http.StatusForbidden)
				}
			}))
			defer server.Close()
			client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			var specification map[string]any
			if err := json.Unmarshal([]byte(test.requestedSpec), &specification); err != nil {
				t.Fatal(err)
			}
			before, err := json.Marshal(specification)
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.ReconcileMetric(t.Context(), tableaupulse.ExpectedMetric{MetricLUID: "metric-1", DefinitionLUID: "definition-1", DatasourceLUID: "datasource-1", SiteLUID: "site-1", Specification: specification})
			after, marshalErr := json.Marshal(specification)
			if marshalErr != nil || string(before) != string(after) {
				t.Fatalf("requested specification changed: before=%s after=%s err=%v", before, after, marshalErr)
			}
			if err != nil || result.Status != test.wantStatus {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if result.Status == "verified" {
				var savedFilter map[string]any
				if err := json.Unmarshal([]byte(test.savedFilter), &savedFilter); err != nil {
					t.Fatal(err)
				}
				wanted, err := json.Marshal([]any{savedFilter})
				if err != nil {
					t.Fatal(err)
				}
				observed, err := json.Marshal(result.Metric.Specification["filters"])
				if err != nil || string(wanted) != string(observed) {
					t.Fatalf("saved filter evidence changed: wanted=%s observed=%s err=%v", wanted, observed, err)
				}
			}
		})
	}
}
