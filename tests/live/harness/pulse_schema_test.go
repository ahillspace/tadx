package harness

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestReadPulseSchemaUsesStrictSiteScopedMetadataRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/vizql-data-service/read-metadata" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Tableau-Site-Id") != "actual-site" {
			t.Errorf("site header = %q", r.Header.Get("X-Tableau-Site-Id"))
		}
		if r.Header.Get("X-Tableau-Auth") != "session" {
			t.Errorf("auth header missing")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(body, map[string]any{"datasource": map[string]any{"datasourceLuid": "target-ds"}}) {
			t.Errorf("body = %#v", body)
		}
		_, _ = w.Write([]byte(`{"data":[{"fieldName":"[Sales]","fieldCaption":"Sales","dataType":"INTEGER","fieldRole":"MEASURE","fieldType":"CONTINUOUS","columnClass":"COLUMN"},{"fieldName":"[Order Date]","fieldCaption":"Order Date","dataType":"DATE","fieldRole":"DIMENSION","fieldType":"ORDINAL","columnClass":"COLUMN"}]}`))
	}))
	t.Cleanup(server.Close)
	client := &Client{Server: server.URL, Version: "3.29", SiteID: "actual-site", token: "session", HTTP: server.Client()}
	schema, err := client.ReadPulseSchema(t.Context(), "target-ds")
	if err != nil {
		t.Fatal(err)
	}
	want := []PulseSchemaField{{ID: "[Sales]", Name: "[Sales]", Caption: "Sales", Role: "measure", DataType: "integer", ColumnClass: "column"}, {ID: "[Order Date]", Name: "[Order Date]", Caption: "Order Date", Role: "dimension", DataType: "date", ColumnClass: "column"}}
	if !reflect.DeepEqual(schema.Fields, want) {
		t.Fatalf("fields = %#v, want %#v", schema.Fields, want)
	}
}

func TestReadPulseSchemaFallsBackToDescribeAndPreservesProviderIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/vizql-data-service/read-metadata":
			w.WriteHeader(http.StatusNotImplemented)
		case "/api/v1/vizql-data-service/describe-datasource":
			_, _ = w.Write([]byte(`{"model":{"fieldGroups":[{"fields":[{"name":"raw-sales-id","caption":"Sales","dataType":"INTEGER","role":"MEASURE","columnClass":"COLUMN"}]}]}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", token: "session", HTTP: server.Client()}
	schema, err := client.ReadPulseSchema(t.Context(), "ds")
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.Fields) != 1 || schema.Fields[0].ID != "raw-sales-id" || schema.Fields[0].Role != "measure" {
		t.Fatalf("schema did not preserve provider identity: %#v", schema)
	}
}

func TestPulseSchemaParsesExplicitEligibilityFlagsAndRejectsMalformedOnes(t *testing.T) {
	for _, test := range []struct {
		name             string
		row              string
		hidden, internal bool
		excluded         bool
	}{
		{name: "documented hidden flag", row: `,"hidden":true`, hidden: true},
		{name: "legacy hidden flag", row: `,"isHidden":true`, hidden: true},
		{name: "internal flag", row: `,"isInternal":true`, internal: true},
		{name: "excluded flag", row: `,"excluded":true`, excluded: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := `{"fieldName":"Sales","dataType":"INTEGER","fieldRole":"MEASURE","columnClass":"COLUMN"`
			row += test.row + `}`
			var document map[string]any
			if err := json.Unmarshal([]byte(`{"data":[`+row+`]}`), &document); err != nil {
				t.Fatal(err)
			}
			fields, err := pulseMetadataFields(document)
			if err != nil {
				t.Fatal(err)
			}
			if len(fields) != 1 || fields[0].Hidden != test.hidden || fields[0].Internal != test.internal || fields[0].Excluded != test.excluded {
				t.Fatalf("parsed field = %#v", fields)
			}
		})
	}
	for _, row := range []string{
		`{"fieldName":"Sales","dataType":"INTEGER","fieldRole":"MEASURE","columnClass":"COLUMN","hidden":"false"}`,
		`{"fieldName":"Sales","dataType":"INTEGER","fieldRole":"MEASURE","columnClass":"COLUMN","hidden":true,"isHidden":false}`,
	} {
		var document map[string]any
		if err := json.Unmarshal([]byte(`{"data":[`+row+`]}`), &document); err != nil {
			t.Fatal(err)
		}
		if _, err := pulseMetadataFields(document); err == nil {
			t.Errorf("invalid visibility facts passed: %s", row)
		}
	}
}

func TestPulseSchemaEndpointDoesNotAllowBroadPOST(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("noncanonical request reached server")
	}))
	t.Cleanup(server.Close)
	client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
	for _, test := range []struct {
		method string
		path   string
		body   []byte
	}{
		{method: http.MethodPost, path: "/api/v1/vizql-data-service/read-metadata", body: []byte(`{"datasource":{"datasourceLuid":"ds"},"extra":"x"}`)},
		{method: http.MethodPost, path: "/api/v1/vizql-data-service/read-metadata?x=1", body: []byte(`{"datasource":{"datasourceLuid":"ds"}}`)},
		{method: http.MethodPost, path: "/api/v1/vizql-data-service/anything", body: []byte(`{"datasource":{"datasourceLuid":"ds"}}`)},
		{method: http.MethodDelete, path: "/api/v1/vizql-data-service/read-metadata", body: []byte(`{"datasource":{"datasourceLuid":"ds"}}`)},
	} {
		if _, err := client.Do(t.Context(), test.method, test.path, test.body, "application/json"); err == nil {
			t.Errorf("noncanonical POST %q was allowed", test.path)
		}
	}
}
