package pulse_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

type session struct{}

func (session) Authorize(request *http.Request) { request.Header.Set("X-Tableau-Auth", "token") }
func (session) SiteLUID() string                { return "site-1" }
func (session) UserLUID() string                { return "user-1" }
func (session) String() string                  { return "session" }

type sessionWithoutSite struct{ session }

func (sessionWithoutSite) SiteLUID() string { return " " }

func newPulseClient(t *testing.T, transport *tableau.Transport, authenticated auth.Session, serverURL string) *tableaupulse.Client {
	t.Helper()
	client, err := tableaupulse.NewClient(transport, authenticated, serverURL)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestListDefinitionsUsesBoundedPulseTokenContract(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/-/pulse/definitions" || request.URL.Query().Get("page_size") != "2" || request.URL.Query().Get("page_token") != "token-1" {
			t.Fatalf("request=%s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		if got := request.Header.Get("X-Tableau-Site-Id"); got != "site-1" {
			t.Fatalf("site header=%q", got)
		}
		writer.Header().Set("X-Tableau-Request-Id", "request-1")
		_, _ = io.WriteString(writer, `{"definitions":[{"metadata":{"id":"definition-1","name":"Revenue","description":"Recognized revenue"},"specification":{"datasource":{"id":"datasource-1"},"basic_specification":{"measure":{"field":"Sales","aggregation":"AGGREGATION_SUM"},"time_dimension":{"field":"Order Date"},"filters":[]}},"extension_options":{"allowed_dimensions":["Region"]}}],"next_page_token":"token-2"}`)
	}))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	page, err := client.ListDefinitions(context.Background(), tableaupulse.PageRequest{PageSize: 2, PageToken: "token-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Definitions) != 1 || page.Definitions[0].LUID != "definition-1" || page.Definitions[0].DatasourceLUID != "datasource-1" || page.NextPageToken != "token-2" || page.TableauRequestID != "request-1" {
		t.Fatalf("page=%#v", page)
	}
}

func TestNewClientRejectsMissingAuthenticatedSiteBeforeNetwork(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := tableaupulse.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), sessionWithoutSite{}, server.URL)
	if err == nil || !strings.Contains(err.Error(), "site LUID") {
		t.Fatalf("err=%v", err)
	}
	if client != nil {
		t.Fatalf("client=%#v", client)
	}
	if calls != 0 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestGetDefinitionAcceptsDocumentedEnvelopeAndPreservesConfiguration(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/-/pulse/definitions/definition-1" {
			t.Fatalf("path=%q", request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"definition":{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"},"basic_specification":{"measure":{"field":"Sales","aggregation":"AGGREGATION_SUM"},"time_dimension":{"field":"Order Date"}}},"extension_options":{"allowed_dimensions":["Region"]}}}`)
	}))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	definition, err := client.GetDefinition(context.Background(), "definition-1")
	if err != nil {
		t.Fatal(err)
	}
	var configuration map[string]any
	if json.Unmarshal(definition.Configuration, &configuration) != nil || definition.MeasureField != "Sales" || configuration["metadata"] == nil {
		t.Fatalf("definition=%#v configuration=%#v", definition, configuration)
	}
}

func TestCreateDefinitionUsesProvenMediaTypesAndResolvesExplicitDefaultMetric(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		switch requests {
		case 1:
			if request.Method != http.MethodPost || request.URL.Path != "/api/-/pulse/definitions" {
				t.Fatalf("request=%s %s", request.Method, request.URL.Path)
			}
			if got := request.Header.Get("Content-Type"); got != "application/vnd.tableau.metricqueryservice.v1.CreateDefinitionRequest+json" {
				t.Fatalf("content type=%q", got)
			}
			if got := request.Header.Get("Accept"); got != "application/vnd.tableau.metricqueryservice.v1.CreateDefinitionResponse+json" {
				t.Fatalf("accept=%q", got)
			}
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if _, exists := body["is_running_total"]; exists {
				t.Fatal("is_running_total leaked to the top level")
			}
			specification := body["specification"].(map[string]any)
			if specification["is_running_total"] != true {
				t.Fatalf("body=%#v", body)
			}
			writer.Header().Set("X-Tableau-Request-Id", "create-request")
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `{"definition":{"metadata":{"id":"definition-1","name":"Revenue"}}}`)
		case 2:
			if request.Method != http.MethodGet || request.URL.Path != "/api/-/pulse/definitions/definition-1/metrics" {
				t.Fatalf("poll=%s %s", request.Method, request.URL.Path)
			}
			writer.Header().Set("X-Tableau-Request-Id", "poll-request")
			_, _ = io.WriteString(writer, `{"metrics":[{"id":"metric-scoped","is_default":false},{"metadata":{"id":"metric-default","name":"default"},"is_default":true}]}`)
		default:
			t.Fatalf("unexpected request %d", requests)
		}
	}))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	input := createRequest()
	falseValue := false
	input.Specification.BasicSpecification.Filters = []tableaupulse.Filter{{Field: "Region", Operator: "IN", CategoricalValues: []tableaupulse.CategoricalValue{{BoolValue: &falseValue}}}}
	wantInput := createRequest()
	wantInput.Specification.BasicSpecification.Filters = []tableaupulse.Filter{{Field: "Region", Operator: "IN", CategoricalValues: []tableaupulse.CategoricalValue{{BoolValue: &falseValue}}}}
	result, err := client.CreateDefinition(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, wantInput) {
		t.Fatalf("client mutated create request: %#v", input)
	}
	if result.DefinitionLUID != "definition-1" || result.DefaultMetricLUID != "metric-default" || result.DefaultMetricStatus != "resolved" || result.TableauRequestID != "create-request" || result.PollRequestID != "poll-request" {
		t.Fatalf("result=%#v", result)
	}
}

func TestCreateDefinitionReturnsCreatedIdentityWhenDefaultMetricPollingTimesOut(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `{"definition":{"metadata":{"id":"definition-1","name":"Revenue"}}}`)
			return
		}
		_, _ = io.WriteString(writer, `{"metrics":[]}`)
	}))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	client.SetPollPolicy(time.Millisecond, 4*time.Millisecond)
	result, err := client.CreateDefinition(context.Background(), createRequest())
	if err == nil || result.DefinitionLUID != "definition-1" || result.DefaultMetricStatus != "pending" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func createRequest() tableaupulse.CreateRequest {
	return tableaupulse.CreateRequest{
		Name: "Revenue", Description: "Recognized revenue",
		Specification: tableaupulse.Specification{
			Datasource:         tableaupulse.Datasource{ID: "datasource-1"},
			BasicSpecification: tableaupulse.BasicSpecification{Measure: tableaupulse.Measure{Field: "Sales", Aggregation: "AGGREGATION_SUM"}, TimeDimension: tableaupulse.TimeDimension{Field: "Order Date"}, Filters: []tableaupulse.Filter{}},
			RunningTotal:       true, Temporality: "TEMPORALITY_OVER_TIME",
		},
		ExtensionOptions:      tableaupulse.ExtensionOptions{AllowedDimensions: []string{"Region"}, AllowedGranularities: []string{"GRANULARITY_BY_DAY"}},
		RepresentationOptions: tableaupulse.RepresentationOptions{Type: "NUMBER_FORMAT_TYPE_CURRENCY", SentimentType: "SENTIMENT_TYPE_UP_IS_GOOD", CurrencyCode: "CURRENCY_CODE_USD"},
		InsightsOptions:       tableaupulse.InsightsOptions{ShowInsights: true, Settings: []tableaupulse.InsightSetting{}},
		Comparisons:           tableaupulse.Comparisons{Comparisons: []tableaupulse.Comparison{}}, DatasourceGoals: []map[string]any{}, RelatedLinks: []map[string]any{}, Certification: tableaupulse.Certification{},
	}
}

func TestMetricContractsPreserveExactSpecificationAndMediaTypes(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		switch requests {
		case 1:
			if request.Method != http.MethodGet || request.URL.Path != "/api/-/pulse/metrics/metric-1" {
				t.Fatalf("get request=%s %s", request.Method, request.URL.Path)
			}
			_, _ = io.WriteString(writer, `{"metric":{"id":"metric-1","definition_id":"definition-1","site_id":"site-1","is_default":false,"specification":{"filters":[],"provider_extension":{"keep":true}}}}`)
		case 2:
			if request.Method != http.MethodPost || request.URL.Path != "/api/-/pulse/metrics:getOrCreate" {
				t.Fatalf("fork request=%s %s", request.Method, request.URL.Path)
			}
			if got := request.Header.Get("Content-Type"); got != "application/vnd.tableau.metricqueryservice.v1.GetOrCreateMetricRequest+json" {
				t.Fatalf("content type=%q", got)
			}
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			specification := body["specification"].(map[string]any)
			if specification["provider_extension"] == nil {
				t.Fatalf("unknown specification field lost: %#v", body)
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `{"metric":{"metadata":{"id":"metric-2","name":"metric_2"}},"is_metric_created":true}`)
		}
	}))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	metric, err := client.GetMetric(context.Background(), "metric-1")
	if err != nil || metric.LUID != "metric-1" || metric.DefinitionLUID != "definition-1" || metric.SiteLUID != "site-1" {
		t.Fatalf("metric=%#v err=%v", metric, err)
	}
	result, err := client.GetOrCreateMetric(context.Background(), tableaupulse.GetOrCreateRequest{DefinitionLUID: metric.DefinitionLUID, Specification: metric.Specification})
	if err != nil || result.MetricLUID != "metric-2" || !result.Created {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestSubscriptionsUseCapturedShapesAndExactDelete(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		switch requests {
		case 1:
			if request.URL.Path != "/api/-/pulse/subscriptions" || request.URL.Query().Get("metric_id") != "metric-1" {
				t.Fatalf("list path=%s query=%s", request.URL.Path, request.URL.RawQuery)
			}
			_, _ = io.WriteString(writer, `{"subscriptions":{"subscription":[{"id":"sub-user","follower":{"user_id":"user-1","name":"User One"}},{"id":"sub-group","subscription":{"follower":{"group":{"id":"group-1","name":"Group One"}}}}]}}`)
		case 2:
			if request.URL.Path != "/api/-/pulse/subscriptions:batchCreate" || request.Method != http.MethodPost {
				t.Fatalf("follow request=%s %s", request.Method, request.URL.Path)
			}
			if got := request.Header.Get("Content-Type"); got != "application/vnd.tableau.pulse.subscriptionservice.v1.BatchCreateSubscriptionsRequest+json" {
				t.Fatalf("content type=%q", got)
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `{"subscriptions":[{"id":"sub-new"}]}`)
		case 3:
			if request.Method != http.MethodDelete || request.URL.Path != "/api/-/pulse/subscriptions/sub-user" {
				t.Fatalf("delete request=%s %s", request.Method, request.URL.Path)
			}
			writer.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	subscriptions, err := client.ListSubscriptions(context.Background(), "metric-1")
	if err != nil || len(subscriptions) != 2 || subscriptions[0].FollowerLUID != "user-1" || subscriptions[1].FollowerType != "GROUP" {
		t.Fatalf("subscriptions=%#v err=%v", subscriptions, err)
	}
	created, err := client.CreateSubscription(context.Background(), tableaupulse.CreateSubscriptionRequest{MetricLUID: "metric-1", FollowerType: "USER", FollowerLUID: "user-1"})
	if err != nil || created.Status != "followed" {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	if err := client.DeleteSubscription(context.Background(), "sub-user"); err != nil {
		t.Fatal(err)
	}
}

func TestGetOrCreateRejectsMissingCreatedFlag(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, `{"metric":{"metadata":{"id":"metric-2"}}}`)
	}))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	_, err := client.GetOrCreateMetric(context.Background(), tableaupulse.GetOrCreateRequest{DefinitionLUID: "definition-1", Specification: map[string]any{"filters": []any{}}})
	if err == nil {
		t.Fatal("missing is_metric_created accepted")
	}
	var protocol interface{ HTTPStatus() int }
	if !errors.As(err, &protocol) {
		t.Fatalf("error lost protocol context: %v", err)
	}
}

func TestListMetricsUsesBoundedDefinitionTokenContract(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/-/pulse/definitions/definition-1/metrics" || request.URL.Query().Get("page_size") != "25" || request.URL.Query().Get("page_token") != "page-1" {
			t.Fatalf("request=%s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		writer.Header().Set("X-Tableau-Request-Id", "request-1")
		_, _ = io.WriteString(writer, `{"metrics":[{"metadata":{"id":"metric-1","name":"Revenue"},"definition_id":"definition-1","is_default":true,"specification":{"filters":[]}}],"next_page_token":"page-2"}`)
	}))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	page, err := client.ListMetrics(context.Background(), "definition-1", tableaupulse.PageRequest{PageSize: 25, PageToken: "page-1"})
	if err != nil || len(page.Metrics) != 1 || page.Metrics[0].LUID != "metric-1" || !page.Metrics[0].IsDefault || page.NextPageToken != "page-2" || page.TableauRequestID != "request-1" {
		t.Fatalf("page=%#v err=%v", page, err)
	}
}

func TestCreateSubscriptionTreatsProvenDuplicateAsConverged(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(writer, `{"error":{"code":"ALREADY_EXISTS","summary":"Subscription already exists for this follower"}}`)
	}))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	result, err := client.CreateSubscription(context.Background(), tableaupulse.CreateSubscriptionRequest{MetricLUID: "metric-1", FollowerType: "GROUP", FollowerLUID: "group-1"})
	if err != nil || result.Status != "already_following" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestReconcileMetricVerifiesExactOwnershipAndSpecification(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		switch requests {
		case 1:
			_, _ = io.WriteString(writer, `{"metric":{"id":"metric-1","definition_id":"definition-1","site_id":"site-1","specification":{"filters":[]}}}`)
		case 2:
			_, _ = io.WriteString(writer, `{"definition":{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"},"basic_specification":{"measure":{"field":"Sales"},"time_dimension":{"field":"Date"}}}}}`)
		default:
			t.Fatalf("unexpected request %d", requests)
		}
	}))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	result, err := client.ReconcileMetric(context.Background(), tableaupulse.ExpectedMetric{MetricLUID: "metric-1", DefinitionLUID: "definition-1", DatasourceLUID: "datasource-1", SiteLUID: "site-1", Specification: map[string]any{"filters": []any{}}})
	if err != nil || result.Status != "verified" || !result.OwnershipVerified || !result.SpecificationVerified || result.Attempts != 1 || requests != 2 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
