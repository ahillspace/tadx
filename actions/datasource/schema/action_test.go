package schema_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	datasourceschema "github.com/ahillspace/tadx/actions/datasource/schema"
	render "github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
)

type reader struct {
	result datasourceschema.Schema
	err    error
	calls  int
	luid   string
}

func (r *reader) ReadDatasourceSchema(_ context.Context, luid string) (datasourceschema.Schema, error) {
	r.calls++
	r.luid = luid
	return r.result, r.err
}

func TestSchemaFiltersAndPaginatesDeterministically(t *testing.T) {
	r := &reader{result: datasourceschema.Schema{
		DatasourceLUID: "ds-1", DatasourceName: "Sales", ObservedAt: "2026-09-04T10:00:00Z", RequestID: "request-1",
		Tables: []datasourceschema.Table{{ID: "orders-id", Name: "Orders", FieldCount: 3}},
		Fields: []datasourceschema.Field{
			{ID: "profit", Caption: "Profit", Table: "Orders", Role: "measure", DataType: "number", DefaultAggregation: "sum"},
			{ID: "sales", Caption: "Revenue Amount", Table: "Orders", Role: "measure", DataType: "number", DefaultAggregation: "sum"},
			{ID: "order_date", Caption: "Order Date", Table: "Orders", Role: "date", DataType: "date"},
		},
	}}
	action := datasourceschema.New(r, func() time.Time { return time.Date(2026, 9, 4, 10, 1, 0, 0, time.UTC) })
	first, err := action.Execute(context.Background(), datasourceschema.Input{Environment: "dev", Site: "site", DatasourceLUID: "ds-1", Query: "amount", Role: "measure", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if r.calls != 1 || r.luid != "ds-1" || first.Page.Total != 1 || first.Page.Returned != 1 || first.Fields[0].ID != "sales" {
		t.Fatalf("unexpected result: calls=%d luid=%q output=%#v", r.calls, r.luid, first)
	}
	if first.Source == nil || first.Source.Mode != readsource.Tableau || first.RequestID != "request-1" {
		t.Fatalf("source/request = %#v / %q", first.Source, first.RequestID)
	}
	compact := first.CompactOutput().(datasourceschema.CompactResult)
	if compact.Fields[0].ID != "sales" || compact.Fields[0].DefaultAggregation != "sum" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
}

func TestSchemaCursorBindsFiltersAndSource(t *testing.T) {
	r := &reader{result: datasourceschema.Schema{DatasourceLUID: "ds-1", DatasourceName: "Sales", Fields: []datasourceschema.Field{{ID: "a", Caption: "A", Role: "measure"}, {ID: "b", Caption: "B", Role: "measure"}}}}
	action := datasourceschema.New(r, time.Now)
	first, err := action.Execute(context.Background(), datasourceschema.Input{Environment: "dev", Site: "site", DatasourceLUID: "ds-1", Role: "measure", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first.Page.NextCursor == "" {
		t.Fatal("expected continuation cursor")
	}
	_, err = action.Execute(context.Background(), datasourceschema.Input{Environment: "dev", Site: "site", DatasourceLUID: "ds-1", Role: "date", Limit: 1, Cursor: first.Page.NextCursor})
	if err == nil {
		t.Fatal("cursor accepted changed filter")
	}
	_, err = action.Execute(context.Background(), datasourceschema.Input{Environment: "dev", Site: "site", DatasourceLUID: "ds-1", Role: "measure", Limit: 1, Cursor: first.Page.NextCursor, Catalog: true})
	if err == nil {
		t.Fatal("cursor accepted changed source")
	}
}

func TestSchemaRejectsInvalidInputBeforeRead(t *testing.T) {
	r := &reader{}
	action := datasourceschema.New(r, time.Now)
	for _, input := range []datasourceschema.Input{
		{},
		{DatasourceLUID: "ds-1", Role: "metric"},
		{DatasourceLUID: "ds-1", Limit: 101},
	} {
		if _, err := action.Execute(context.Background(), input); err == nil {
			t.Fatalf("accepted %#v", input)
		}
	}
	if r.calls != 0 {
		t.Fatalf("reader calls = %d", r.calls)
	}
}

func TestSchemaCatalogSourceIsHonest(t *testing.T) {
	r := &reader{result: datasourceschema.Schema{DatasourceLUID: "ds-1", DatasourceName: "Sales", ObservedAt: "2026-09-04T10:00:00Z", Fields: []datasourceschema.Field{{ID: "a", Caption: "A", Role: "measure"}}}}
	action := datasourceschema.New(r, func() time.Time { return time.Date(2026, 9, 4, 10, 1, 0, 0, time.UTC) })
	out, err := action.Execute(context.Background(), datasourceschema.Input{Environment: "dev", Site: "site", DatasourceLUID: "ds-1", Catalog: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source == nil || out.Source.Mode != readsource.Catalog {
		t.Fatalf("source = %#v", out.Source)
	}
	// The action has no generation provenance, so the unwrapped placeholder must
	// never claim complete, fresh coverage.
	if out.Source.Coverage != readsource.CoveragePartial || !out.Source.Stale || out.Source.GenerationID != "" {
		t.Fatalf("dishonest catalog placeholder: %#v", out.Source)
	}
}

func TestSchemaOutputGolden(t *testing.T) {
	observed := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	source := readsource.Live(observed)
	output := datasourceschema.Output{
		Status: "listed", Environment: "dev", Site: "sandbox",
		DatasourceLUID: "datasource-1", DatasourceName: "Sales",
		Tables: []datasourceschema.Table{{ID: "orders-id", Name: "Orders", FieldCount: 2}},
		Page:   datasourceschema.Page{Returned: 2, Total: 2, Limit: 20},
		Fields: []datasourceschema.Field{
			{ID: "order_date", Name: "order_date", Caption: "Order Date", Table: "Orders", Role: "date", DataType: "date", TimeType: "date"},
			{ID: "sales", Name: "sales", Caption: "Revenue Amount", Table: "Orders", Role: "measure", DataType: "number", DefaultAggregation: "sum"},
		},
		Source:    &source,
		RequestID: "request-1",
		Help:      []string{"tadx pulse definition create -h"},
	}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
}

func assertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

func TestSchemaWrapsReadFailure(t *testing.T) {
	r := &reader{err: errors.New("metadata unavailable")}
	_, err := datasourceschema.New(r, time.Now).Execute(context.Background(), datasourceschema.Input{Environment: "dev", Site: "site", DatasourceLUID: "ds-1"})
	if err == nil || err.Error() == "metadata unavailable" {
		t.Fatalf("error = %v", err)
	}
}
