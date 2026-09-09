package catalog

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTolerantInventoryCountsMalformedRowsAcrossPagesButRunRemainsStrict(t *testing.T) {
	for _, test := range []struct {
		scope            Scope
		malformed, valid string
	}{
		{ScopeWorkbooks, `<workbook id="bad" name="Bad" size="large"/>`, `<workbook id="good" name="Good"/>`},
		{ScopeDatasources, `<datasource id="bad" name="Bad" hasExtracts="sometimes"/>`, `<datasource id="good" name="Good"/>`},
		{ScopeUsers, `<user id="bad"/>`, `<user id="good" name="Good"/>`},
	} {
		t.Run(string(test.scope), func(t *testing.T) {
			executor := executorFunc(func(_ context.Context, request Request) (Response, error) {
				if request.Scope == ScopeProjects {
					return Response{StatusCode: 200, Body: []byte(listXML("projects", "project", 1, 1, 0, ""))}, nil
				}
				item := test.malformed
				if request.PageNumber == 2 {
					item = test.valid
				}
				definition := collectors[test.scope]
				return Response{StatusCode: 200, Body: []byte(listXML(definition.container, definition.item, request.PageNumber, 1, 2, item))}, nil
			})
			engine, err := NewEngine(executor, Config{PageSize: 1})
			if err != nil {
				t.Fatal(err)
			}
			got, err := engine.CollectInventory(context.Background(), test.scope, InventoryOptions{SkipMalformedRecords: true})
			if err != nil || got.Total != 1 || got.SkippedRows != 1 || len(got.Rows) != 1 || got.Rows[0][0] != "good" {
				t.Fatalf("inventory = %#v %v", got, err)
			}
			if _, err := engine.CollectInventory(context.Background(), test.scope); err == nil {
				t.Fatal("strict inventory accepted malformed row")
			}
			_, err = engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{test.scope}}, &inventoryWriter{rows: make(map[Scope][][]any)})
			if err == nil || !strings.Contains(err.Error(), "invalid Tableau") {
				t.Fatalf("strict catalog Run error = %v", err)
			}
		})
	}
}

func TestTolerantInventoryStillRejectsDuplicateMalformedIdentity(t *testing.T) {
	engine, err := NewEngine(executorFunc(func(_ context.Context, request Request) (Response, error) {
		return Response{StatusCode: 200, Body: []byte(listXML("users", "user", 1, 2, 2, `<user id="same"/><user id="same" name="Valid"/>`))}, nil
	}), Config{PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.CollectInventory(context.Background(), ScopeUsers, InventoryOptions{SkipMalformedRecords: true}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate malformed identity error = %v", err)
	}
}

func TestInventoryFilterAppliesToEveryRootPageOnly(t *testing.T) {
	const filter = "name:eq:Selected"
	engine, err := NewEngine(executorFunc(func(_ context.Context, request Request) (Response, error) {
		if request.Scope == ScopeProjects {
			if got := request.Query.Get("filter"); got != "" {
				t.Errorf("dependency received filter %q", got)
			}
			return Response{StatusCode: 200, Body: []byte(listXML("projects", "project", 1, 1, 0, ""))}, nil
		}
		if got := request.Query.Get("filter"); got != filter {
			t.Errorf("page%d filter=%q", request.PageNumber, got)
		}
		item := `<workbook id="first" name="Selected"/>`
		if request.PageNumber == 2 {
			item = `<workbook id="second" name="Selected"/>`
		}
		return Response{StatusCode: 200, Body: []byte(listXML("workbooks", "workbook", request.PageNumber, 1, 2, item))}, nil
	}), Config{PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	got, err := engine.CollectInventory(context.Background(), ScopeWorkbooks, InventoryOptions{Filter: filter})
	if err != nil {
		t.Fatal(err)
	}
	if got.Filter != filter || len(got.Rows) != 2 || got.Requests != 3 {
		t.Fatalf("snapshot=%+v", got)
	}
}

func TestFilteredProjectInventoryRetainsUnfilteredHierarchy(t *testing.T) {
	engine, err := NewEngine(executorFunc(func(_ context.Context, req Request) (Response, error) {
		if req.Query.Get("filter") != "" {
			return Response{StatusCode: 200, TableauRequestID: "filtered", Body: []byte(listXML("projects", "project", 1, 1, 1, `<project id="child" name="Selected" parentProjectId="parent"/>`))}, nil
		}
		item := `<project id="parent" name="Parent"/>`
		if req.PageNumber == 2 {
			item = `<project id="child" name="Selected" parentProjectId="parent"/>`
		}
		return Response{StatusCode: 200, TableauRequestID: "hierarchy", Body: []byte(listXML("projects", "project", req.PageNumber, 1, 2, item))}, nil
	}), Config{PageSize: 1, InitialConcurrency: 1, MaxConcurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	got, err := engine.CollectInventory(context.Background(), ScopeProjects, InventoryOptions{Filter: "name:eq:Selected"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Rows) != 1 || got.Rows[0][0] != "child" || len(got.Dependencies) != 1 || got.Dependencies[0].Scope != ScopeProjects || len(got.Dependencies[0].Rows) != 2 {
		t.Fatalf("snapshot=%+v", got)
	}
	if got.Requests != 3 || len(got.TableauRequestIDs) != 2 || got.FinalConcurrency != 3 {
		t.Fatalf("metadata=%+v", got)
	}
}

func TestInventoryMaxRowsRejectsRootBeforePagination(t *testing.T) {
	var calls atomic.Int32
	engine, _ := NewEngine(executorFunc(func(_ context.Context, req Request) (Response, error) {
		calls.Add(1)
		return Response{StatusCode: 200, Body: []byte(listXML("users", "user", req.PageNumber, 1, 10001, `<user id="u1" name="First"/>`))}, nil
	}), Config{PageSize: 1})
	_, err := engine.CollectInventory(context.Background(), ScopeUsers, InventoryOptions{MaxRows: 10000})
	if err == nil || !strings.Contains(err.Error(), "10000") || calls.Load() != 1 {
		t.Fatalf("calls=%d error=%v", calls.Load(), err)
	}
}

func TestInventoryMaxRowsDoesNotCapPrerequisitePopulation(t *testing.T) {
	engine, _ := NewEngine(executorFunc(func(_ context.Context, req Request) (Response, error) {
		if req.Scope == ScopeProjects {
			return Response{StatusCode: 200, Body: []byte(listXML("projects", "project", 1, 1000, 2, `<project id="p1" name="One"/><project id="p2" name="Two"/>`))}, nil
		}
		return Response{StatusCode: 200, Body: []byte(listXML("workbooks", "workbook", 1, 1000, 1, `<workbook id="w1" name="One"/>`))}, nil
	}), Config{})
	got, err := engine.CollectInventory(context.Background(), ScopeWorkbooks, InventoryOptions{MaxRows: 1})
	if err != nil || got.Total != 1 || len(got.Dependencies) != 1 || len(got.Dependencies[0].Rows) != 2 {
		t.Fatalf("snapshot=%+v error=%v", got, err)
	}
}
