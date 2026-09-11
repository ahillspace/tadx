package cache

import (
	"encoding/json"
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
		{scope: ScopeUsers, item: `<user id="u1" name="Alice" email="alice@example.com" siteRole="Explorer" lastLogin="2026-01-01T00:00:00Z"/>`, want: []any{"u1", "Alice", "alice@example.com", "Explorer", "2026-01-01T00:00:00Z"}},
		{scope: ScopeGroups, item: `<group id="g1" name="Analysts"><domain name="local"/></group>`, want: []any{"g1", "Analysts", "local"}},
		{scope: ScopeProjects, item: `<project id="p1" name="Operations" parentProjectId="root" description="Ops"><owner id="u1"/></project>`, want: []any{"p1", "Operations", "root", "Ops", "u1"}},
		{scope: ScopeWorkbooks, item: `<workbook id="w1" name="Sales" size="42" updatedAt="2026-01-02T00:00:00Z"><project id="p1"/><owner id="u1"/></workbook>`, want: []any{"w1", "Sales", "p1", "u1", int64(42), "2026-01-02T00:00:00Z"}},
		{scope: ScopeDatasources, item: `<datasource id="d1" name="Orders" updatedAt="2026-01-03T00:00:00Z"><project id="p1"/><owner id="u1"/></datasource>`, want: []any{"d1", "Orders", "p1", "u1", "2026-01-03T00:00:00Z"}},
		{scope: ScopeFlows, item: `<flow id="f1" name="Prep" fileType="tflx" updatedAt="2026-01-04T00:00:00Z"><project id="p1"/><owner id="u1"/></flow>`, want: []any{"f1", "Prep", "p1", "u1", "tflx", "2026-01-04T00:00:00Z"}},
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
			if len(parsed.rows) != 1 {
				t.Fatalf("rows = %#v; want one row", parsed.rows)
			}
			row := parsed.rows[0]
			if test.scope != ScopeViews {
				if len(row) != len(test.want)+1 {
					t.Fatalf("row width = %d; want %d", len(row), len(test.want)+1)
				}
				var payload map[string]any
				if err := json.Unmarshal([]byte(row[len(row)-1].(string)), &payload); err != nil || payload["luid"] != test.want[0] {
					t.Fatalf("list payload = %#v, error = %v", payload, err)
				}
				row = row[:len(row)-1]
			}
			if fmt.Sprint(row) != fmt.Sprint(test.want) {
				t.Fatalf("rows = %#v; want %#v", parsed.rows, test.want)
			}
			columns, _ := ColumnsForScope(test.scope)
			if len(parsed.rows[0]) != len(columns) {
				t.Fatalf("row width = %d; columns = %d", len(parsed.rows[0]), len(columns))
			}
		})
	}
}

func TestCompleteCollectorsPreserveExistingListProjection(t *testing.T) {
	tests := []struct {
		scope Scope
		item  string
		want  map[string]any
	}{
		{ScopeUsers, `<user id="u1" name="alice" fullName="Alice A" email="alice@example.com" siteRole="Creator" lastLogin="2026-01-01T00:00:00Z" authSetting="SAML"><domain name="local"/></user>`, map[string]any{"email": "alice@example.com", "full_name": "Alice A", "auth_setting": "SAML", "domain": "local"}},
		{ScopeGroups, `<group id="g1" name="Authors" minimumSiteRole="Explorer" externalUserEnabled="true"><domain name="local"/><import grantLicenseMode="onLogin" siteRole="Viewer"/></group>`, map[string]any{"domain": "local", "minimum_site_role": "Explorer", "grant_license_mode": "onLogin", "external_user_enabled": true}},
		{ScopeProjects, `<project id="p1" name="Ops" description="Operations" topLevelProject="false" contentPermissions="LockedToProject" controllingPermissionsProjectId="root" createdAt="2026-01-01T00:00:00Z" updatedAt="2026-01-02T00:00:00Z"><owner id="u1"/><contentCounts projectCount="1" workbookCount="2" viewCount="3" datasourceCount="4"/></project>`, map[string]any{"content_permissions": "LockedToProject", "controlling_permissions_project_luid": "root", "project_count": float64(1), "datasource_count": float64(4)}},
		{ScopeWorkbooks, `<workbook id="w1" name="Sales" contentUrl="sales" description="Sales reporting" createdAt="2026-01-01T00:00:00Z" updatedAt="2026-01-02T00:00:00Z"><project id="p1" name="Ops"/><owner id="u1"/><tags><tag label="daily"/><tag label="certified"/></tags></workbook>`, map[string]any{"content_url": "sales", "description": "Sales reporting", "created_at": "2026-01-01T00:00:00Z", "tags": []any{"certified", "daily"}}},
		{ScopeDatasources, `<datasource id="d1" name="Orders" type="hyper" contentUrl="orders" description="Orders data" size="42" encryptExtracts="false" hasExtracts="true" isCertified="true" certificationNote="Reviewed" useRemoteQueryAgent="false" webpageUrl="https://example.test/d1" createdAt="2026-01-01T00:00:00Z" updatedAt="2026-01-02T00:00:00Z"><project id="p1" name="Ops"/><owner id="u1"/><tags><tag label="daily"/></tags><askData enablement="Enabled"/></datasource>`, map[string]any{"type": "hyper", "content_url": "orders", "size": float64(42), "has_extracts": true, "certification_note": "Reviewed", "ask_data_enablement": "Enabled", "tags": []any{"daily"}}},
		{ScopeFlows, `<flow id="f1" name="Prep" fileType="tflx" description="Daily prep" createdAt="2026-01-01T00:00:00Z" updatedAt="2026-01-02T00:00:00Z"><project id="p1" name="Ops"/><owner id="u1"/><tags><tag label="daily"/></tags></flow>`, map[string]any{"file_type": "tflx", "description": "Daily prep", "created_at": "2026-01-01T00:00:00Z", "tags": []any{"daily"}}},
	}
	for _, test := range tests {
		t.Run(string(test.scope), func(t *testing.T) {
			definition := collectors[test.scope]
			parsed, err := parseList(definition, []byte(listXML(definition.container, definition.item, 1, 1000, 1, test.item)))
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(parsed.rows[0][len(parsed.rows[0])-1].(string)), &payload); err != nil {
				t.Fatal(err)
			}
			for key, want := range test.want {
				if fmt.Sprint(payload[key]) != fmt.Sprint(want) {
					t.Fatalf("payload[%q] = %#v, want %#v", key, payload[key], want)
				}
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
