package refresh_test

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	refresh "github.com/ahillspace/tadx/actions/catalog/refresh"
	"github.com/ahillspace/tadx/internal/output"
)

type inventory struct{ snapshot refresh.Snapshot }

func (i inventory) Read(context.Context, refresh.Input) (refresh.Snapshot, error) {
	return i.snapshot, nil
}

type recordingInventory struct {
	inputs []refresh.Input
}

func (i *recordingInventory) Read(_ context.Context, input refresh.Input) (refresh.Snapshot, error) {
	i.inputs = append(i.inputs, input)
	return refresh.Snapshot{GeneratedAt: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), Source: "tableau-rest", Records: []refresh.Record{}}, nil
}

type writer struct{ result refresh.WriteResult }

func (w writer) Replace(context.Context, refresh.Generation) (refresh.WriteResult, error) {
	return w.result, nil
}

func TestActionDefaultsAndValidatesExactScopesBeforeInventory(t *testing.T) {
	inventory := &recordingInventory{}
	action := refresh.New(inventory, writer{result: refresh.WriteResult{}})
	if _, err := action.Execute(context.Background(), refresh.Input{Environment: "production", SiteResolved: true}); err != nil {
		t.Fatal(err)
	}
	want := []string{"projects", "workbooks", "datasources", "flows"}
	if len(inventory.inputs) != 1 || !reflect.DeepEqual(inventory.inputs[0].Scopes, want) {
		t.Fatalf("inventory inputs = %#v", inventory.inputs)
	}
	for _, scopes := range [][]string{{"project"}, {"projects", "projects"}, {"Projects"}} {
		before := len(inventory.inputs)
		if _, err := action.Execute(context.Background(), refresh.Input{Environment: "production", SiteResolved: true, Scopes: scopes}); err == nil {
			t.Fatalf("Execute() accepted scopes %#v", scopes)
		}
		if len(inventory.inputs) != before {
			t.Fatalf("inventory called for scopes %#v", scopes)
		}
	}
}

func TestActionCompactAndFullOutput(t *testing.T) {
	action := refresh.New(inventory{snapshot: refresh.Snapshot{
		GeneratedAt: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), Source: "tableau-rest",
		Records: []refresh.Record{{LUID: "wb-1", Kind: "workbook", Name: "Finance"}},
	}}, writer{result: refresh.WriteResult{GenerationID: "generation-1", Path: "catalog/production.json", RecordCount: 1}})
	for _, test := range []struct {
		name, golden string
		full         bool
	}{{"compact", "testdata/compact.toon", false}, {"full", "testdata/full.toon", true}} {
		t.Run(test.name, func(t *testing.T) {
			value, err := action.Execute(context.Background(), refresh.Input{Environment: "production", Site: "marketing", SiteResolved: true, Scopes: []string{"workbooks"}})
			if err != nil {
				t.Fatal(err)
			}
			assertGolden(t, test.golden, value, test.full)
		})
	}
}

func assertGolden(t *testing.T, path string, value any, full bool) {
	t.Helper()
	var actual bytes.Buffer
	if err := output.RenderWithOptions(&actual, value, output.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected = bytes.TrimSuffix(expected, []byte("\n"))
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual.Bytes())
	}
}
