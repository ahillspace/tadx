package catalog

import (
	"fmt"
	"strings"
	"testing"
)

func TestCollectorsProduceFixedTypedRows(t *testing.T) {
	tests := []struct {
		scope Scope
		item  string
		want  []any
	}{
		{scope: ScopeUsers, item: `<user id="u1" name="Alice" siteRole="Explorer" lastLogin="2026-01-01T00:00:00Z"><email>alice@example.com</email></user>`, want: []any{"u1", "Alice", "alice@example.com", "Explorer", "2026-01-01T00:00:00Z"}},
		{scope: ScopeGroups, item: `<group id="g1" name="Analysts" domain="local"/>`, want: []any{"g1", "Analysts", "local"}},
		{scope: ScopeProjects, item: `<project id="p1" name="Operations" parentProjectId="root" description="Ops"><owner id="u1"/></project>`, want: []any{"p1", "Operations", "root", "Ops", "u1"}},
		{scope: ScopeWorkbooks, item: `<workbook id="w1" name="Sales" size="42" updatedAt="2026-01-02T00:00:00Z"><project id="p1"/><owner id="u1"/></workbook>`, want: []any{"w1", "Sales", "p1", "u1", int64(42), "2026-01-02T00:00:00Z"}},
		{scope: ScopeDatasources, item: `<datasource id="d1" name="Orders" updatedAt="2026-01-03T00:00:00Z"><project id="p1"/><owner id="u1"/></datasource>`, want: []any{"d1", "Orders", "p1", "u1", "2026-01-03T00:00:00Z"}},
		{scope: ScopeFlows, item: `<flow id="f1" name="Prep" updatedAt="2026-01-04T00:00:00Z"><project id="p1"/><owner id="u1"/></flow>`, want: []any{"f1", "Prep", "p1", "u1", "2026-01-04T00:00:00Z"}},
		{scope: ScopeViews, item: `<view id="v1" name="Dashboard"><workbook id="w1"/></view>`, want: []any{"v1", "Dashboard", "w1"}},
	}
	for _, test := range tests {
		t.Run(string(test.scope), func(t *testing.T) {
			definition := collectors[test.scope]
			body := []byte(listXML(definition.container, definition.item, 1, 1000, 1, test.item))
			parsed, err := parseList(definition, body)
			if err != nil {
				t.Fatal(err)
			}
			if len(parsed.rows) != 1 || fmt.Sprint(parsed.rows[0]) != fmt.Sprint(test.want) {
				t.Fatalf("rows = %#v; want %#v", parsed.rows, test.want)
			}
			columns, _ := ColumnsForScope(test.scope)
			if len(parsed.rows[0]) != len(columns) {
				t.Fatalf("row width = %d; columns = %d", len(parsed.rows[0]), len(columns))
			}
		})
	}
}

func TestCollectorsRejectIncompleteAndMalformedRows(t *testing.T) {
	definition := collectors[ScopeWorkbooks]
	tests := []struct {
		item string
		want string
	}{
		{item: `<workbook name="No ID"/>`, want: "incomplete authoritative identity"},
		{item: `<workbook id="w1"/>`, want: "incomplete authoritative identity"},
		{item: `<workbook id="w1" name="One" size="large"/>`, want: "nonnegative integer"},
		{item: `<workbook id="w1" name="One" size="-1"/>`, want: "nonnegative integer"},
	}
	for _, test := range tests {
		body := []byte(listXML(definition.container, definition.item, 1, 1000, 1, test.item))
		_, err := parseList(definition, body)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestPermissionParserRejectsMismatchedWorkbookIdentity(t *testing.T) {
	body := []byte(`<tsResponse><permissions><workbook id="different"/></permissions></tsResponse>`)
	_, _, err := parsePermissions("expected", body)
	if err == nil || !strings.Contains(err.Error(), `expected workbook "expected"`) {
		t.Fatalf("error = %v", err)
	}
}
