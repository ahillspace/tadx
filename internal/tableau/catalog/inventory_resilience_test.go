package catalog

import (
	"context"
	"strings"
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
