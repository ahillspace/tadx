package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ahillspace/tadx/internal/app"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDatasourceMetadataThroughCLI(t *testing.T) {
	var calls, graphs atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if diagnosticSignIn(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/3.29/auth/signout":
			w.WriteHeader(204)
		case "/api/3.29/sites/site-1/datasources/ds-1":
			io.WriteString(w, `<tsResponse><datasource id="ds-1" name="Sales"><project id="p1" name="Operations"/></datasource></tsResponse>`)
		case "/api/3.29/sites/site-1/projects":
			io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="p1" name="Operations"/></projects></tsResponse>`)
		case "/api/v1/vizql-data-service/read-metadata":
			io.WriteString(w, `{"data":[{"fieldName":"[Sales]","fieldCaption":"Revenue","dataType":"REAL","fieldRole":"MEASURE","defaultAggregation":"SUM","logicalTableId":"Orders"},{"fieldName":"[Cost]","fieldCaption":"Revenue","dataType":"REAL","fieldRole":"MEASURE","logicalTableId":"Orders"}]}`)
		case "/api/metadata/graphql":
			graphs.Add(1)
			var request struct {
				Query string `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			node := `{"__typename":"DatabaseServer","id":"db-meta","luid":"db-1","name":"Warehouse","description":"Database details","connectionType":"postgres"}`
			if strings.Contains(request.Query, "items:upstreamTablesConnection") {
				node = `{"__typename":"DatabaseTable","id":"table-meta","luid":"table-1","name":"orders","description":"Table details","database":{"id":"db-meta","luid":"db-1","name":"Warehouse","__typename":"DatabaseServer"}}`
			}
			if strings.Contains(request.Query, "items:fieldsConnection") {
				node = `{"id":"field-meta","name":"Revenue","fullyQualifiedName":"[Sales]","description":"Published meaning","descriptionInherited":[{"value":"Physical meaning","assetId":"column-meta","attribute":"description","distance":1,"edges":[]}],"upstreamColumnsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"column-meta","luid":"column-1","name":"sales","description":"Physical meaning","table":{"id":"table-meta","luid":"table-1","name":"orders","__typename":"DatabaseTable"},"tagsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"name":"finance"}]}}]}}`
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"data":{"parents":{"nodes":[{"id":"ds-meta","luid":"ds-1","name":"Sales","description":"Sales data","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[%s]}}]}}}`, node)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	options := diagnosticOptions(t, server)
	run := func(args ...string) string {
		t.Helper()
		var out strings.Builder
		args = append(args, "--env", "test", "--json")
		if code := app.Run(context.Background(), args, &out, options); code != 0 {
			t.Fatalf("exit %d: %s", code, out.String())
		}
		return out.String()
	}
	compact := run("content", "datasource", "inspect", "--id", "ds-1")
	if !strings.Contains(compact, "db-meta") || !strings.Contains(compact, "table-1") || strings.Contains(compact, "Table details") {
		t.Fatalf("compact upstream: %s", compact)
	}
	first := graphs.Load()
	full := run("content", "datasource", "inspect", "--id", "ds-1", "--full")
	if graphs.Load()-first != first || !strings.Contains(full, "Table details") {
		t.Fatalf("full changed queries or omitted descriptions: %s", full)
	}
	before := calls.Load()
	run("content", "datasource", "inspect", "--id", "ds-1", "--cache", "--full")
	if calls.Load() != before {
		t.Fatal("cached inspect used Tableau")
	}
	schema := run("content", "datasource", "schema", "--id", "ds-1", "--descriptions", "--tags", "--field-id", "[Sales]")
	for _, want := range []string{"Published meaning", "Physical meaning", "column-meta", "finance", "[Sales]"} {
		if !strings.Contains(schema, want) {
			t.Fatalf("missing %s: %s", want, schema)
		}
	}
	if strings.Contains(schema, "[Cost]") {
		t.Fatalf("unselected field leaked: %s", schema)
	}
	before = calls.Load()
	cached := run("content", "datasource", "schema", "--id", "ds-1", "--descriptions", "--tags", "--cache", "--field-id", "[Sales]", "--full")
	if calls.Load() != before || !strings.Contains(cached, "Published meaning") {
		t.Fatalf("cache enrichment lost: %s", cached)
	}
	plain := run("content", "datasource", "schema", "--id", "ds-1", "--cache", "--full")
	if strings.Contains(plain, "Published meaning") {
		t.Fatalf("optional metadata emitted unrequested: %s", plain)
	}
	run("content", "datasource", "schema", "--id", "ds-1")
	before = calls.Load()
	preserved := run("content", "datasource", "schema", "--id", "ds-1", "--descriptions", "--cache", "--field-id", "[Sales]")
	if calls.Load() != before || !strings.Contains(preserved, "Published meaning") {
		t.Fatalf("plain schema erased metadata observation: %s", preserved)
	}
}
