package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
)

func TestEngineSkipsOnlyExplicitPersonalSpacePermissions(t *testing.T) {
	for _, test := range []struct {
		name, location, project string
		skip                    bool
	}{
		{name: "personal space", location: `<location type="PersonalSpace" id="space"/>`, skip: true},
		{name: "personal space empty project", location: `<location type="PersonalSpace"/>`, project: `<project id=" "/>`, skip: true},
		{name: "project", location: `<location type="Project" id="p1"/>`, project: `<project id="p1"/>`},
		{name: "missing location and project"},
		{name: "missing location with project", project: `<project id="p1"/>`},
		{name: "empty location", location: `<location/>`},
		{name: "unknown location", location: `<location type="Other"/>`},
		{name: "case mismatch", location: `<location type="personalspace"/>`},
		{name: "conflicting project", location: `<location type="PersonalSpace"/>`, project: `<project id="p1"/>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var permissionCalls atomic.Int32
			item := `<workbook id="w1" name="Draft" contentUrl="draft" description="Retained description"><owner id="u1"/><tags><tag label="retained"/></tags>` + test.location + test.project + `</workbook>`
			engine, err := NewEngine(executorFunc(func(_ context.Context, request Request) (Response, error) {
				switch request.Scope {
				case ScopeProjects:
					return Response{StatusCode: http.StatusOK, Body: []byte(listXML("projects", "project", 1, 1000, 1, `<project id="p1" name="Project"/>`))}, nil
				case ScopeWorkbooks:
					return Response{StatusCode: http.StatusOK, Body: []byte(listXML("workbooks", "workbook", 1, 1000, 1, item))}, nil
				case ScopePermissions:
					permissionCalls.Add(1)
					if request.ItemID != "w1" {
						t.Errorf("permission workbook = %q, want w1", request.ItemID)
					}
					return Response{StatusCode: http.StatusForbidden, TableauRequestID: "denied-w1"}, nil
				default:
					return Response{}, fmt.Errorf("unexpected scope %q", request.Scope)
				}
			}), Config{})
			if err != nil {
				t.Fatal(err)
			}
			writer := &memoryWriter{}
			result, err := engine.Run(t.Context(), RunRequest{RequestedScopes: []Scope{ScopePermissions}}, writer)
			if err != nil {
				t.Fatal(err)
			}
			wantCalls := int32(1)
			if test.skip {
				wantCalls = 0
			}
			if permissionCalls.Load() != wantCalls || result.DeniedPermissions != int(wantCalls) {
				t.Errorf("permission calls = %d, denied = %d, want %d", permissionCalls.Load(), result.DeniedPermissions, wantCalls)
			}
			rows := writer.rows(ScopeWorkbooks)
			if result.Counts[ScopeWorkbooks] != 1 || len(rows) != 1 || len(rows[0]) != 7 || rows[0][0] != "w1" || rows[0][1] != "Draft" || rows[0][3] != "u1" {
				t.Fatalf("workbook count = %d, rows = %#v", result.Counts[ScopeWorkbooks], rows)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(rows[0][6].(string)), &payload); err != nil {
				t.Fatal(err)
			}
			if payload["luid"] != "w1" || payload["name"] != "Draft" || payload["content_url"] != "draft" || payload["description"] != "Retained description" || fmt.Sprint(payload["tags"]) != "[retained]" {
				t.Fatalf("workbook payload lost data: %#v", payload)
			}
		})
	}
}

func TestEnginePersonalSpacePermissionsAcrossPages(t *testing.T) {
	items := []string{
		`<workbook id="personal-first" name="First"><location type="PersonalSpace"/></workbook>`,
		`<workbook id="project" name="Project"><location type="Project" id="p1"/><project id="p1"/></workbook>`,
		`<workbook id="unknown" name="Unknown"><location type="Other"/></workbook>`,
		`<workbook id="personal-last" name="Last"><location type="PersonalSpace"/></workbook>`,
	}
	var mu sync.Mutex
	pages := make(map[int]int)
	permissionCalls := make(map[string]int)
	engine, err := NewEngine(executorFunc(func(_ context.Context, request Request) (Response, error) {
		switch request.Scope {
		case ScopeProjects:
			return Response{StatusCode: http.StatusOK, Body: []byte(listXML("projects", "project", 1, 1, 1, `<project id="p1" name="Project"/>`))}, nil
		case ScopeWorkbooks:
			mu.Lock()
			pages[request.PageNumber]++
			mu.Unlock()
			if request.PageNumber < 1 || request.PageNumber > len(items) || request.PageSize != 1 {
				return Response{}, fmt.Errorf("unexpected workbook pagination: %#v", request)
			}
			return Response{StatusCode: http.StatusOK, Body: []byte(listXML("workbooks", "workbook", request.PageNumber, 1, len(items), items[request.PageNumber-1]))}, nil
		case ScopePermissions:
			mu.Lock()
			permissionCalls[request.ItemID]++
			mu.Unlock()
			if request.ItemID != "project" && request.ItemID != "unknown" {
				return Response{StatusCode: http.StatusForbidden}, nil
			}
			body := fmt.Sprintf(`<tsResponse><permissions><workbook id="%s"/><granteeCapabilities><group id="g1"/><capabilities><capability name="Read" mode="Allow"/></capabilities></granteeCapabilities></permissions></tsResponse>`, request.ItemID)
			return Response{StatusCode: http.StatusOK, Body: []byte(body)}, nil
		default:
			return Response{}, fmt.Errorf("unexpected scope %q", request.Scope)
		}
	}), Config{PageSize: 1, MaxConcurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	writer := &memoryWriter{}
	result, err := engine.Run(t.Context(), RunRequest{RequestedScopes: []Scope{ScopePermissions}}, writer)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != len(items) {
		t.Errorf("workbook pages = %#v", pages)
	}
	for page, calls := range pages {
		if page < 1 || page > len(items) || calls != 1 {
			t.Errorf("workbook page %d requested %d times", page, calls)
		}
	}
	if !slices.Equal(slices.Sorted(maps.Keys(permissionCalls)), []string{"project", "unknown"}) || permissionCalls["project"] != 1 || permissionCalls["unknown"] != 1 {
		t.Errorf("permission calls = %#v", permissionCalls)
	}
	if result.Requests != 7 || result.DeniedPermissions != 0 || result.Counts[ScopeWorkbooks] != 4 || result.Counts[ScopePermissions] != 2 || len(writer.rows(ScopePermissions)) != 2 {
		t.Errorf("result = %#v", result)
	}
	var workbookIDs []string
	for _, row := range writer.rows(ScopeWorkbooks) {
		workbookIDs = append(workbookIDs, row[0].(string))
	}
	slices.Sort(workbookIDs)
	if !slices.Equal(workbookIDs, []string{"personal-first", "personal-last", "project", "unknown"}) {
		t.Fatalf("retained workbook identities = %v", workbookIDs)
	}
}

func TestCollectInventoryRetainsPersonalSpaceWorkbooks(t *testing.T) {
	engine, err := NewEngine(executorFunc(func(_ context.Context, request Request) (Response, error) {
		switch request.Scope {
		case ScopeProjects:
			return Response{StatusCode: http.StatusOK, Body: []byte(listXML("projects", "project", 1, 1000, 0, ""))}, nil
		case ScopeWorkbooks:
			return Response{StatusCode: http.StatusOK, Body: []byte(listXML("workbooks", "workbook", 1, 1000, 1, `<workbook id="personal" name="Draft" contentUrl="draft"><location type="PersonalSpace"/></workbook>`))}, nil
		default:
			return Response{}, fmt.Errorf("unexpected inventory scope %q", request.Scope)
		}
	}), Config{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := engine.CollectInventory(t.Context(), ScopeWorkbooks)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Total != 1 || snapshot.SkippedRows != 0 || len(snapshot.Rows) != 1 || snapshot.Rows[0][0] != "personal" || snapshot.Rows[0][1] != "Draft" || snapshot.Rows[0][2] != "" {
		t.Fatalf("personal workbook inventory = %#v", snapshot)
	}
}
