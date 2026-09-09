package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	definitioncreate "github.com/ahillspace/tadx/actions/pulse/definition/create"
	"github.com/ahillspace/tadx/internal/app"
)

func TestPulseCaptionForkThroughCLI(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		name := "caption and raw ID combine values"
		if mixed {
			name = "caption and raw ID reject mixed operators"
		}
		t.Run(name, func(t *testing.T) {
			writes := 0
			var saved map[string]any
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/3.29/auth/signin":
					io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case "/api/-/pulse/metrics/source":
					io.WriteString(w, `{"id":"source","definition_id":"definition-1","specification":{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[]}}`)
				case "/api/-/pulse/definitions/definition-1":
					io.WriteString(w, `{"metadata":{"id":"definition-1","name":"Managers"},"specification":{"datasource":{"id":"datasource-1"}},"extension_options":{"allowed_dimensions":["People"],"allowed_granularities":["GRANULARITY_BY_DAY"]}}`)
				case "/api/3.29/sites/site-1/datasources/datasource-1":
					io.WriteString(w, `<tsResponse><datasource id="datasource-1" name="Orders"><project id="project-1" name="Test"/></datasource></tsResponse>`)
				case "/api/v1/vizql-data-service/read-metadata":
					io.WriteString(w, `{"data":[{"fieldName":"People","fieldCaption":"Regional Manager","dataType":"STRING","fieldRole":"DIMENSION"},{"fieldName":"OtherPeople","fieldCaption":"People","dataType":"STRING","fieldRole":"DIMENSION"}]}`)
				case "/api/-/pulse/metrics:getOrCreate":
					writes++
					var body struct {
						Specification map[string]any `json:"specification"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					saved = body.Specification
					io.WriteString(w, `{"metric":{"id":"variant-1"},"is_metric_created":true}`)
				case "/api/-/pulse/metrics/variant-1":
					json.NewEncoder(w).Encode(map[string]any{"id": "variant-1", "definition_id": "definition-1", "site_id": "site-1", "specification": saved})
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			options := pulseEfficiencyOptions(t, server)
			args := []string{"pulse", "metric", "fork", "--environment", "test", "--id", "source", "--filter", "Regional Manager=Alice", "--filter", "Regional Manager= Regional Manager ", "--full"}
			flag := "--filter"
			if mixed {
				flag = "--exclude-filter"
			}
			args = append(args, flag, "People=Bob")
			for _, preview := range []bool{true, false} {
				invocation := append([]string(nil), args...)
				opts := options
				if preview {
					invocation = append(invocation, "--preview")
					opts = disabledPreviewOptions(opts)
				}
				var out bytes.Buffer
				code := app.Run(context.Background(), invocation, &out, opts)
				if mixed {
					if code == 0 || writes != 0 || !strings.Contains(out.String(), "conflicting include and exclude filters") {
						t.Fatalf("mixed operators preview=%t code=%d writes=%d output=%s", preview, code, writes, out.String())
					}
					continue
				}
				if code != 0 {
					t.Fatalf("preview=%t code=%d output=%s", preview, code, out.String())
				}
				if preview {
					if writes != 0 || !strings.Contains(out.String(), "People") || !strings.Contains(out.String(), "Alice") || !strings.Contains(out.String(), "Bob") {
						t.Fatalf("preview writes=%d output=%s", writes, out.String())
					}
				} else {
					want := []any{map[string]any{"field": "People", "operator": "OPERATOR_EQUAL", "categorical_values": []any{map[string]any{"string_value": " Regional Manager "}, map[string]any{"string_value": "Alice"}, map[string]any{"string_value": "Bob"}}, "include_null": false}}
					if writes != 1 || !reflect.DeepEqual(saved["filters"], want) {
						t.Fatalf("writes=%d filters=%#v", writes, saved["filters"])
					}
				}
			}
		})
	}
}

// Exercise the CLI, live metadata adapter, validation, and provider payload together.
func TestPulseCaptionPublishingThroughCLI(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		name := "unique captions resolve to raw IDs"
		if ambiguous {
			name = "ambiguous caption rejects preview and execution"
		}
		t.Run(name, func(t *testing.T) {
			creates := 0
			var created definitioncreate.CreateRequest
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/auth/signin":
					io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/datasources/datasource-1":
					io.WriteString(w, `<tsResponse><datasource id="datasource-1" name="Orders"><project id="project-1" name="Test"/></datasource></tsResponse>`)
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/vizql-data-service/read-metadata":
					extra := ""
					if ambiguous {
						extra = `,{"fieldName":"OtherPeople","fieldCaption":"Regional Manager","dataType":"STRING","fieldRole":"DIMENSION"}`
					}
					io.WriteString(w, `{"data":[{"fieldName":"People","fieldCaption":"Regional Manager","dataType":"STRING","fieldRole":"DIMENSION"},{"fieldName":"OrderDate","fieldCaption":"Order Date","dataType":"DATE","fieldRole":"DIMENSION"},{"fieldName":"Territory","fieldCaption":"Sales Region","dataType":"STRING","fieldRole":"DIMENSION"}`+extra+`]}`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/-/pulse/definitions":
					io.WriteString(w, `{"definitions":[]}`)
				case r.Method == http.MethodPost && r.URL.Path == "/api/-/pulse/definitions":
					creates++
					if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
						t.Error(err)
					}
					w.WriteHeader(http.StatusCreated)
					io.WriteString(w, `{"definition":{"metadata":{"id":"definition-1"}}}`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/-/pulse/definitions/definition-1/metrics":
					io.WriteString(w, `{"metrics":[{"metadata":{"id":"metric-1"},"definition_id":"definition-1","is_default":true,"specification":{}}]}`)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			options := pulseEfficiencyOptions(t, server)
			args := []string{"pulse", "definition", "create", "--environment", "test", "--name", "Managers", "--datasource-id", "datasource-1", "--measure-field", "Regional Manager", "--aggregation", "COUNT_DISTINCT", "--date-field", "Order Date", "--dimension", "Sales Region", "--full"}
			for _, preview := range []bool{true, false} {
				invocation := append([]string(nil), args...)
				opts := options
				if preview {
					invocation = append(invocation, "--preview")
					opts = disabledPreviewOptions(opts)
				}
				var stdout bytes.Buffer
				code := app.Run(context.Background(), invocation, &stdout, opts)
				if ambiguous {
					if code == 0 || creates != 0 || !strings.Contains(strings.ToLower(stdout.String()), "ambiguous") {
						t.Fatalf("preview=%t code=%d creates=%d output=%s", preview, code, creates, stdout.String())
					}
					continue
				}
				if code != 0 {
					t.Fatalf("preview=%t code=%d output=%s", preview, code, stdout.String())
				}
				if preview {
					if creates != 0 || !strings.Contains(stdout.String(), "People") || !strings.Contains(stdout.String(), "OrderDate") || !strings.Contains(stdout.String(), "Territory") {
						t.Fatalf("preview must expose canonical fields without writes: creates=%d output=%s", creates, stdout.String())
					}
				} else if creates != 1 || created.Specification.BasicSpecification.Measure.Field != "People" || created.Specification.BasicSpecification.TimeDimension.Field != "OrderDate" || !reflect.DeepEqual(created.ExtensionOptions.AllowedDimensions, []string{"Territory"}) {
					t.Fatalf("wrong publish payload: creates=%d request=%+v", creates, created)
				}
			}
		})
	}
}
