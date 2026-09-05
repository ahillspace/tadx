package fieldcatalog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
)

type session struct{}

func (session) Authorize(request *http.Request) { request.Header.Set("X-Tableau-Auth", "token") }
func (session) SiteLUID() string                { return "site-1" }
func (session) UserLUID() string                { return "user-1" }
func (session) String() string                  { return "[redacted session]" }

func TestClientUsesVDSReadMetadataAndPreservesRawFieldIdentity(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/vizql-data-service/read-metadata" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-Tableau-Auth") != "token" || request.Header.Get("X-Tableau-Site-Id") != "site-1" {
			t.Fatalf("auth headers = %#v", request.Header)
		}
		var body struct {
			Datasource struct {
				LUID string `json:"datasourceLuid"`
			} `json:"datasource"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.Datasource.LUID != "ds-1" {
			t.Fatalf("body = %#v, err = %v", body, err)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Tableau-Request-Id", "schema-request")
		_, _ = writer.Write([]byte(`{"data":[{"fieldName":"Calculation_123","fieldCaption":"Revenue","dataType":"REAL","fieldRole":"MEASURE","defaultAggregation":"AGG","columnClass":"CALCULATION","logicalTableId":"Orders_A1B2C3D4"},{"fieldName":"Calculation_456","fieldCaption":"Flat Fee","dataType":"INTEGER","fieldRole":"MEASURE","defaultAggregation":"SUM","columnClass":"CALCULATION","logicalTableId":"Orders_A1B2C3D4"},{"fieldName":"Order Date","fieldCaption":"Order Date","dataType":"DATE","fieldRole":"DIMENSION","logicalTableId":"Orders_A1B2C3D4"},{"fieldName":"Rank","fieldCaption":"Rank","dataType":"INTEGER","fieldRole":"MEASURE","columnClass":"TABLE_CALCULATION"}]}`))
	}))
	defer server.Close()
	client := fieldcatalog.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	got, err := client.Read(context.Background(), "ds-1", "Sales")
	if err != nil {
		t.Fatal(err)
	}
	if got.RequestID != "schema-request" || got.DatasourceLUID != "ds-1" || got.DatasourceName != "Sales" || len(got.Tables) != 1 || len(got.Fields) != 4 {
		t.Fatalf("schema = %#v", got)
	}
	foundAggregate, foundRowLevel, foundDate, foundExcluded := false, false, false, false
	for _, field := range got.Fields {
		foundAggregate = foundAggregate || field.ID == "Calculation_123" && field.Caption == "Revenue" && field.DefaultAggregation == "AGG" && field.RequiresUserAggregation
		foundRowLevel = foundRowLevel || field.ID == "Calculation_456" && field.Caption == "Flat Fee" && field.DefaultAggregation == "SUM" && !field.RequiresUserAggregation
		foundDate = foundDate || field.ID == "Order Date" && field.Role == "date" && field.TimeType == "DATE"
		foundExcluded = foundExcluded || field.ID == "Rank" && field.Excluded && field.ExclusionReason == "table_calc"
	}
	if !foundAggregate || !foundRowLevel || !foundDate || !foundExcluded {
		t.Fatalf("fields = %#v", got.Fields)
	}
}

func TestClientFallsBackToDescribeDatasource(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/vizql-data-service/read-metadata" {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":{"code":"404","summary":"Unavailable"}}`))
			return
		}
		if request.URL.Path != "/api/v1/vizql-data-service/describe-datasource" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"datasourceModel":{"logicalTables":[{"logicalTableId":"orders","caption":"Orders"}]},"fieldGroups":[{"logicalTableId":"orders","fields":[{"name":"Sales","caption":"Sales","dataType":"REAL","role":"MEASURE","defaultAggregation":"SUM"}]}]}`))
	}))
	defer server.Close()
	client := fieldcatalog.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	got, err := client.Read(context.Background(), "ds-1", "Sales")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(got.Fields) != 1 || got.Fields[0].ID != "Sales" || got.Fields[0].Table != "Orders" {
		t.Fatalf("requests=%d schema=%#v", requests, got)
	}
}

func TestClientFallsBackToMetadataGraphQLAndPreservesCalculatedFieldID(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/vizql-data-service/read-metadata", "/api/v1/vizql-data-service/describe-datasource":
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":{"code":"404","summary":"Unavailable"}}`))
		case "/api/metadata/graphql":
			writer.Header().Set("X-Tableau-Request-Id", "metadata-request")
			_, _ = writer.Write([]byte(`{"data":{"publishedDatasources":[{"luid":"ds-1","name":"Sales","fieldsConnection":{"totalCount":2,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[{"__typename":"CalculatedField","id":"meta-1","name":"Revenue Ratio","fullyQualifiedName":"[Calculation_42]","role":"MEASURE","dataType":"REAL","aggregation":"AGG","formula":"SUM([Sales]) / SUM([Target])","isHidden":false},{"__typename":"ColumnField","id":"meta-2","name":"Order Date","fullyQualifiedName":"[Order Date]","role":"DIMENSION","dataType":"DATE","aggregation":"YEAR","isHidden":false}]}}]}}`))
		default:
			t.Fatalf("path = %q", request.URL.Path)
		}
	}))
	defer server.Close()
	client := fieldcatalog.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	got, err := client.Read(context.Background(), "ds-1", "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 || got.RequestID != "metadata-request" || got.DatasourceName != "Sales" || len(got.Fields) != 2 {
		t.Fatalf("requests=%d schema=%#v", requests, got)
	}
	foundCalc := false
	for _, field := range got.Fields {
		foundCalc = foundCalc || field.ID == "Calculation_42" && field.Caption == "Revenue Ratio" && field.RequiresUserAggregation && field.Provenance == "metadata_graphql"
	}
	if !foundCalc {
		t.Fatalf("fields = %#v", got.Fields)
	}
}
