package datasource_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	render "github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type schemaReader struct {
	result datasourceops.SchemaRecord
	err    error
	calls  int
	luid   string
}

func TestSchemaSchemaAllInventoryBoundAndConflicts(t *testing.T) {
	r := &schemaReader{result: datasourceops.SchemaRecord{DatasourceLUID: "ds-1", DatasourceName: "Orders"}}
	for index := 0; index < 10001; index++ {
		r.result.Fields = append(r.result.Fields, datasourceops.Field{ID: fmt.Sprint(index), Caption: fmt.Sprint(index), Role: "dimension"})
	}
	actionReader, actionNow := r, time.Now
	if _, err := datasourceops.Schema(context.Background(), actionReader, actionNow, datasourceops.SchemaInput{DatasourceLUID: "ds-1", All: true}); err == nil {
		t.Fatal("oversized all inventory accepted")
	}
	r.result.Fields = r.result.Fields[:10000]
	out, err := datasourceops.Schema(context.Background(), actionReader, actionNow, datasourceops.SchemaInput{DatasourceLUID: "ds-1", All: true})
	if err != nil || out.Page.Returned != 10000 || out.Page.MoreAvailable {
		t.Fatalf("page=%+v err=%v", out.Page, err)
	}
	for _, input := range []datasourceops.SchemaInput{{DatasourceLUID: "ds-1", All: true, Limit: 20}, {DatasourceLUID: "ds-1", All: true, Cursor: "legacy"}} {
		before := r.calls
		if _, err := datasourceops.Schema(context.Background(), actionReader, actionNow, input); err == nil || r.calls != before {
			t.Fatalf("err=%v calls=%d", err, r.calls)
		}
	}
}

func TestSchemaSchemaRequestedLimitMatchesAllBound(t *testing.T) {
	r := &schemaReader{result: datasourceops.SchemaRecord{DatasourceLUID: "ds-1", DatasourceName: "Orders"}}
	for index := 0; index < 10000; index++ {
		r.result.Fields = append(r.result.Fields, datasourceops.Field{ID: fmt.Sprint(index), Caption: fmt.Sprint(index), Role: "dimension"})
	}
	for _, limit := range []int{1000, 10000} {
		out, err := datasourceops.Schema(context.Background(), r, time.Now, datasourceops.SchemaInput{DatasourceLUID: "ds-1", Limit: limit})
		if err != nil || out.Page.Returned != limit || out.Page.MoreAvailable != (limit < 10000) {
			t.Fatalf("limit=%d page=%+v err=%v", limit, out.Page, err)
		}
	}
}

func (r *schemaReader) ReadDatasourceSchema(_ context.Context, luid string) (datasourceops.SchemaRecord, error) {
	r.calls++
	r.luid = luid
	return r.result, r.err
}

func TestSchemaSchemaFiltersAndPaginatesDeterministically(t *testing.T) {
	r := &schemaReader{result: datasourceops.SchemaRecord{
		DatasourceLUID: "ds-1", DatasourceName: "Sales", ObservedAt: "2026-09-04T10:00:00Z", RequestID: "request-1",
		Tables: []datasourceops.Table{{ID: "orders-id", Name: "Orders", FieldCount: 3}},
		Fields: []datasourceops.Field{
			{ID: "profit", Caption: "Profit", Table: "Orders", Role: "measure", DataType: "number", DefaultAggregation: "sum"},
			{ID: "sales", Caption: "Revenue Amount", Table: "Orders", Role: "measure", DataType: "number", DefaultAggregation: "sum"},
			{ID: "order_date", Caption: "Order Date", Table: "Orders", Role: "date", DataType: "date"},
		},
	}}
	actionReader, actionNow := r, func() time.Time { return time.Date(2026, 9, 4, 10, 1, 0, 0, time.UTC) }
	first, err := datasourceops.Schema(context.Background(), actionReader, actionNow, datasourceops.SchemaInput{Environment: "dev", Site: "site", DatasourceLUID: "ds-1", Query: "amount", Role: "measure", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if r.calls != 1 || r.luid != "ds-1" || first.Page.Total != 1 || first.Page.Returned != 1 || first.Fields[0].ID != "sales" {
		t.Fatalf("unexpected result: calls=%d luid=%q output=%#v", r.calls, r.luid, first)
	}
	if first.Source == nil || first.Source.Mode != readsource.Tableau || first.RequestID != "request-1" {
		t.Fatalf("source/request = %#v / %q", first.Source, first.RequestID)
	}
	compact := first.CompactOutput().(datasourceops.SchemaCompactResult)
	if compact.Fields[0].ID != "sales" || compact.Fields[0].DefaultAggregation != "sum" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
}

func TestSchemaSchemaCursorBindsFiltersAndSource(t *testing.T) {
	r := &schemaReader{result: datasourceops.SchemaRecord{DatasourceLUID: "ds-1", DatasourceName: "Sales", Fields: []datasourceops.Field{{ID: "a", Caption: "A", Role: "measure"}, {ID: "b", Caption: "B", Role: "measure"}}}}
	actionReader, actionNow := r, time.Now
	first, err := datasourceops.Schema(context.Background(), actionReader, actionNow, datasourceops.SchemaInput{Environment: "dev", Site: "site", DatasourceLUID: "ds-1", Role: "measure", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first.Page.NextCursor == "" {
		t.Fatal("expected continuation cursor")
	}
	_, err = datasourceops.Schema(context.Background(), actionReader, actionNow, datasourceops.SchemaInput{Environment: "dev", Site: "site", DatasourceLUID: "ds-1", Role: "date", Limit: 1, Cursor: first.Page.NextCursor})
	if err == nil {
		t.Fatal("cursor accepted changed filter")
	}
	_, err = datasourceops.Schema(context.Background(), actionReader, actionNow, datasourceops.SchemaInput{Environment: "dev", Site: "site", DatasourceLUID: "ds-1", Role: "measure", Limit: 1, Cursor: first.Page.NextCursor, Cache: true})
	if err == nil {
		t.Fatal("cursor accepted changed source")
	}
}

func TestSchemaSchemaRejectsInvalidInputBeforeRead(t *testing.T) {
	r := &schemaReader{}
	actionReader, actionNow := r, time.Now
	for _, input := range []datasourceops.SchemaInput{
		{},
		{DatasourceLUID: "ds-1", Role: "metric"},
		{DatasourceLUID: "ds-1", Limit: 10001},
	} {
		if _, err := datasourceops.Schema(context.Background(), actionReader, actionNow, input); err == nil {
			t.Fatalf("accepted %#v", input)
		}
	}
	if r.calls != 0 {
		t.Fatalf("reader calls = %d", r.calls)
	}
}

func TestSchemaSchemaCacheSourceIsHonest(t *testing.T) {
	r := &schemaReader{result: datasourceops.SchemaRecord{DatasourceLUID: "ds-1", DatasourceName: "Sales", ObservedAt: "2026-09-04T10:00:00Z", Fields: []datasourceops.Field{{ID: "a", Caption: "A", Role: "measure"}}}}
	actionReader, actionNow := r, func() time.Time { return time.Date(2026, 9, 4, 10, 1, 0, 0, time.UTC) }
	out, err := datasourceops.Schema(context.Background(), actionReader, actionNow, datasourceops.SchemaInput{Environment: "dev", Site: "site", DatasourceLUID: "ds-1", Cache: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source == nil || out.Source.Mode != readsource.Cache {
		t.Fatalf("source = %#v", out.Source)
	}
	// The action has no generation provenance, so the unwrapped placeholder must
	// never claim complete, fresh coverage.
	if out.Source.Coverage != readsource.CoveragePartial || !out.Source.Stale || out.Source.GenerationID != "" {
		t.Fatalf("dishonest cache placeholder: %#v", out.Source)
	}
}

func TestSchemaSchemaOutputGolden(t *testing.T) {
	observed := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	source := readsource.Live(observed)
	output := datasourceops.SchemaOutput{
		Status: "listed", Environment: "dev", Site: "sandbox",
		DatasourceLUID: "datasource-1", DatasourceName: "Sales",
		Tables: []datasourceops.Table{{ID: "orders-id", Name: "Orders", FieldCount: 2}},
		Page:   datasourceops.SchemaPage{Returned: 2, Total: 2, Limit: 20},
		Fields: []datasourceops.Field{
			{ID: "order_date", Name: "order_date", Caption: "Order Date", Table: "Orders", Role: "date", DataType: "date", TimeType: "date"},
			{ID: "sales", Name: "sales", Caption: "Revenue Amount", Table: "Orders", Role: "measure", DataType: "number", DefaultAggregation: "sum"},
		},
		Source:    &source,
		RequestID: "request-1",
	}
	schemaAssertGolden(t, "compact.toon", output, false)
	schemaAssertGolden(t, "full.toon", output, true)
}

func schemaAssertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata/schema", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

func TestSchemaSchemaWrapsReadFailure(t *testing.T) {
	r := &schemaReader{err: errors.New("metadata unavailable")}
	_, err := datasourceops.Schema(context.Background(), r, time.Now, datasourceops.SchemaInput{Environment: "dev", Site: "site", DatasourceLUID: "ds-1"})
	if err == nil || err.Error() == "metadata unavailable" {
		t.Fatalf("error = %v", err)
	}
}

func TestSchemaSchemaQueryMatchesFieldMetadataAndKeepsTableFilterSeparate(t *testing.T) {
	r := &schemaReader{result: datasourceops.SchemaRecord{DatasourceLUID: "ds-1", DatasourceName: "Sales", Fields: []datasourceops.Field{
		{ID: "sales-id", Caption: "By identity", Table: "Sales"},
		{ID: "name", Name: "sales-name", Caption: "By name", Table: "Sales"},
		{ID: "caption", Caption: "Sales caption", Table: "Sales"},
		{ID: "label", Caption: "By label", Label: "Sales label", Table: "Sales"},
		{ID: "formula", Caption: "By formula", Formula: "SUM([Sales])", Table: "Sales"},
		{ID: "profit", Caption: "Profit", Table: "Sales"},
		{ID: "other", Caption: "Sales elsewhere", Table: "Other"},
	}}}
	out, err := datasourceops.Schema(context.Background(), r, time.Now, datasourceops.SchemaInput{DatasourceLUID: "ds-1", Query: "sAlEs", Table: "Sales"})
	if err != nil || out.Page.Total != 5 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	for _, field := range out.Fields {
		if field.ID == "profit" || field.ID == "other" {
			t.Fatalf("unrelated field included: %+v", field)
		}
	}
	unfiltered, err := datasourceops.Schema(context.Background(), r, time.Now, datasourceops.SchemaInput{DatasourceLUID: "ds-1", Table: "Sales"})
	if err != nil || unfiltered.Page.Total != 6 {
		t.Fatalf("table filter changed: out=%+v err=%v", unfiltered, err)
	}
}
