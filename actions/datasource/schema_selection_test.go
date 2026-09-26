package datasource_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/errs"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSchemaSchemaExactSelectionAndErrors(t *testing.T) {
	for _, test := range []struct {
		name    string
		ids     []string
		table   string
		want    []string
		errorID string
	}{
		{name: "literal commas and whitespace", ids: []string{"b,c", " a "}, want: []string{" a ", "b,c"}},
		{name: "duplicates form a set", ids: []string{"b,c", "b,c"}, want: []string{"b,c"}},
		{name: "missing is not silently omitted", ids: []string{"b,c", "missing"}, errorID: "datasource.schema.field_not_found"},
		{name: "case is exact", ids: []string{"B,c"}, errorID: "datasource.schema.field_not_found"},
		{name: "ambiguous requires selection", ids: []string{"duplicate"}, errorID: "datasource.schema.field_ambiguous"},
		{name: "table disambiguates", ids: []string{"duplicate"}, table: "Orders", want: []string{"duplicate"}},
		{name: "conflicting filter fails", ids: []string{"b,c"}, table: "Customers", errorID: "datasource.schema.field_not_found"},
		{name: "empty fails", ids: []string{""}, errorID: "datasource.schema.usage"},
		{name: "blank fails", ids: []string{"  "}, errorID: "datasource.schema.usage"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := &schemaReader{result: datasourceops.SchemaRecord{DatasourceLUID: "ds-1", DatasourceName: "Sales", Fields: []datasourceops.Field{
				{ID: " a ", Caption: "A", Table: "Orders"},
				{ID: "b,c", Caption: "B", Table: "Orders"},
				{ID: "duplicate", Caption: "D", Table: "Orders"},
				{ID: "duplicate", Caption: "D", Table: "Customers"},
			}}}
			out, err := datasourceops.Schema(context.Background(), r, nil, datasourceops.SchemaInput{DatasourceLUID: "ds-1", FieldIDs: test.ids, Table: test.table})
			if test.errorID != "" {
				var diagnostic *errs.Error
				if !errors.As(err, &diagnostic) || diagnostic.ID != test.errorID {
					t.Fatalf("error=%v; want %s", err, test.errorID)
				}
				if test.errorID == "datasource.schema.usage" && r.calls != 0 {
					t.Fatalf("invalid selection read schema %d times", r.calls)
				}
				return
			}
			var got []string
			for _, field := range out.Fields {
				got = append(got, field.ID)
			}
			if err != nil || !reflect.DeepEqual(got, test.want) || r.calls != 1 {
				t.Fatalf("fields=%v calls=%d err=%v", got, r.calls, err)
			}
		})
	}
}

func TestSchemaSchemaCursorBindsEntireSelectionSet(t *testing.T) {
	r := &schemaReader{result: datasourceops.SchemaRecord{DatasourceLUID: "ds-1", DatasourceName: "Sales", Fields: []datasourceops.Field{
		{ID: "a", Caption: "A"}, {ID: "b", Caption: "B"}, {ID: "c", Caption: "C"},
	}}}
	actionReader, actionNow := r, (func() time.Time)(nil)
	input := datasourceops.SchemaInput{DatasourceLUID: "ds-1", FieldIDs: []string{"a", "b"}, Limit: 1}
	first, err := datasourceops.Schema(context.Background(), actionReader, actionNow, input)
	if err != nil || first.Page.NextCursor == "" || first.Fields[0].ID != "a" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	input.Cursor = first.Page.NextCursor
	input.FieldIDs = []string{"b", "a", "a"}
	second, err := datasourceops.Schema(context.Background(), actionReader, actionNow, input)
	if err != nil || len(second.Fields) != 1 || second.Fields[0].ID != "b" || second.Page.MoreAvailable {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	for _, ids := range [][]string{{"a", "c"}, {"a"}, nil} {
		input.FieldIDs = ids
		before := r.calls
		_, err := datasourceops.Schema(context.Background(), actionReader, actionNow, input)
		var diagnostic *errs.Error
		if !errors.As(err, &diagnostic) || diagnostic.ID != "datasource.schema.cursor" || r.calls != before {
			t.Fatalf("changed selection %v: err=%v calls=%d/%d", ids, err, before, r.calls)
		}
	}
}

func TestSchemaSchemaSingleSelectorPreservesLegacyCursorIdentity(t *testing.T) {
	r := &schemaReader{result: datasourceops.SchemaRecord{DatasourceLUID: "ds-1", DatasourceName: "Sales", Fields: []datasourceops.Field{{ID: "a", Caption: "A"}}}}
	fingerprint := sha256.Sum256([]byte(strings.Join([]string{"dev", "site", "ds-1", "", "", "", "a", "false"}, "\x00")))
	raw, err := json.Marshal(map[string]any{"offset": 0, "fingerprint": fmt.Sprintf("%x", fingerprint[:])})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []datasourceops.SchemaInput{{FieldID: "a"}, {FieldIDs: []string{"a"}}} {
		input.Environment, input.Site, input.DatasourceLUID = "dev", "site", "ds-1"
		input.Cursor = base64.RawURLEncoding.EncodeToString(raw)
		out, err := datasourceops.Schema(context.Background(), r, nil, input)
		if err != nil || len(out.Fields) != 1 || out.Fields[0].ID != "a" {
			t.Fatalf("out=%+v err=%v", out, err)
		}
	}
}
