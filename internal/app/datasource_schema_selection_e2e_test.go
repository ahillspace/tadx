package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/toon"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRepeatedDatasourceFieldSelectionThroughCLI(t *testing.T) {
	server, calls := schemaSelectionServer(t)
	defer server.Close()
	options := diagnosticOptions(t, server)
	selection := []string{"content", "datasource", "schema", "--environment", "test", "--id", "ds-1", "--field-id", "Sales, net", "--field-id", "Order Date", "--field-id", "Segment calculation"}
	output := runSchemaSelection(t, options, append(append([]string(nil), selection...), "--full")...)
	if calls.identity.Load() != 1 || calls.schema.Load() != 1 {
		t.Fatalf("exact selection repeated upstream reads: identity=%d schema=%d", calls.identity.Load(), calls.schema.Load())
	}
	if output.Page.Returned != 3 || output.Page.Total != 3 || output.Page.MoreAvailable || len(output.Fields) != 3 {
		t.Fatalf("repeated selection lost fields: %#v", output)
	}
	fields := map[string]datasourceops.Field{}
	for _, field := range output.Fields {
		fields[field.ID] = field
	}
	if fields["Sales, net"].Role != "measure" || fields["Order Date"].Role != "date" || fields["Segment calculation"].Formula != `IF [Sales, net] > 100 THEN 'Large' ELSE 'Small' END` {
		t.Fatalf("full selection lost identity, roles, or formula: %#v", fields)
	}
	if _, found := fields["Profit"]; found {
		t.Fatal("full output included an unrequested field")
	}
	before := calls.total.Load()
	cached := runSchemaSelection(t, options, append(append([]string(nil), selection...), "--cache", "--full")...)
	if calls.total.Load() != before || cached.Page.Returned != 3 || cached.Source == nil || cached.Source.Mode != "cache" {
		t.Fatalf("cache selection: calls=%d/%d output=%#v", before, calls.total.Load(), cached)
	}
	for _, test := range []struct{ id, want string }{{"Unknown", "datasource.schema.field_not_found"}, {"Duplicate", "datasource.schema.field_ambiguous"}} {
		var diagnostic bytes.Buffer
		exit := app.Run(context.Background(), []string{"content", "datasource", "schema", "--environment", "test", "--id", "ds-1", "--field-id", test.id, "--cache"}, &diagnostic, options)
		if exit != 2 || !strings.Contains(diagnostic.String(), test.want) || calls.total.Load() != before {
			t.Fatalf("cached selection %q: exit=%d calls=%d/%d output=%s", test.id, exit, before, calls.total.Load(), diagnostic.String())
		}
	}
	duplicate := runSchemaSelection(t, options, append(append([]string(nil), selection...), "--field-id", "Sales, net", "--full")...)
	if duplicate.Page.Returned != 3 || len(duplicate.Fields) != 3 {
		t.Fatalf("duplicate selectors duplicated or replaced fields: %#v", duplicate)
	}
	limited := runSchemaSelection(t, options, append(append([]string(nil), selection...), "--limit", "1", "--full")...)
	if limited.Page.Returned != 1 || limited.Page.Total != 3 || !limited.Page.MoreAvailable {
		t.Fatalf("bounded selection concealed truncation: %#v", limited.Page)
	}
	all := runSchemaSelection(t, options, append(append([]string(nil), selection...), "--all", "--full")...)
	if all.Page.Returned != 3 || all.Page.MoreAvailable {
		t.Fatalf("complete selection was not exhausted: %#v", all.Page)
	}
}

func TestExactDatasourceFieldSelectionErrorsThroughCLI(t *testing.T) {
	for _, test := range []struct {
		name  string
		flags []string
		want  string
	}{
		{"unknown", []string{"--field-id", "Sales, net", "--field-id", "Unknown"}, "datasource.schema.field_not_found"},
		{"ambiguous", []string{"--field-id", "Duplicate"}, "datasource.schema.field_ambiguous"},
		{"empty", []string{"--field-id", ""}, "datasource.schema.usage"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, calls := schemaSelectionServer(t)
			defer server.Close()
			options := diagnosticOptions(t, server)
			args := append([]string{"content", "datasource", "schema", "--environment", "test", "--id", "ds-1"}, test.flags...)
			var output bytes.Buffer
			exit := app.Run(context.Background(), args, &output, options)
			if exit != 2 || !strings.Contains(output.String(), test.want) || calls.identity.Load() > 1 || calls.schema.Load() > 1 {
				t.Fatalf("exit=%d identity=%d schema=%d output=%s", exit, calls.identity.Load(), calls.schema.Load(), output.String())
			}
		})
	}
}

type schemaSelectionCalls struct{ total, identity, schema atomic.Int32 }

func schemaSelectionServer(t *testing.T) (*httptest.Server, *schemaSelectionCalls) {
	t.Helper()
	calls := &schemaSelectionCalls{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.total.Add(1)
		if diagnosticSignIn(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/3.29/auth/signout":
			w.WriteHeader(http.StatusNoContent)
		case "/api/3.29/sites/site-1/datasources/ds-1":
			calls.identity.Add(1)
			_, _ = io.WriteString(w, `<tsResponse><datasource id="ds-1" name="Sales"><project id="p1" name="Orders"/></datasource></tsResponse>`)
		case "/api/v1/vizql-data-service/read-metadata":
			calls.schema.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[
{"fieldName":"Sales, net","fieldCaption":"Sales","dataType":"REAL","fieldRole":"MEASURE","defaultAggregation":"SUM","logicalTableId":"Orders"},
{"fieldName":"Order Date","fieldCaption":"Order Date","dataType":"DATE","fieldRole":"DIMENSION","logicalTableId":"Orders"},
{"fieldName":"Segment calculation","fieldCaption":"Segment","dataType":"STRING","fieldRole":"DIMENSION","columnClass":"CALCULATION","formula":"IF [Sales, net] > 100 THEN 'Large' ELSE 'Small' END","logicalTableId":"Orders"},
{"fieldName":"Profit","fieldCaption":"Profit","dataType":"REAL","fieldRole":"MEASURE","logicalTableId":"Orders"},
{"fieldName":"Duplicate","fieldCaption":"Same name","dataType":"STRING","fieldRole":"DIMENSION","logicalTableId":"Orders"},
{"fieldName":"Duplicate","fieldCaption":"Same name","dataType":"STRING","fieldRole":"DIMENSION","logicalTableId":"Customers"}
]}`)
		default:
			t.Errorf("schema selection made an unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	return server, calls
}

func runSchemaSelection(t *testing.T, options app.Options, args ...string) datasourceops.SchemaFullResult {
	t.Helper()
	output := runGroupOneCLI(t, options, args...)
	decoded, err := toon.Decode([]byte(output))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	var result datasourceops.SchemaFullResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
