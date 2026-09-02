package clean_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	workspaceclean "github.com/ahillspace/tadx/actions/workspace/clean"
	render "github.com/ahillspace/tadx/internal/output"
)

type cleaner struct {
	request workspaceclean.Request
	result  workspaceclean.Result
	err     error
}

func (c *cleaner) Clean(_ context.Context, request workspaceclean.Request) (workspaceclean.Result, error) {
	c.request = request
	return c.result, c.err
}

func TestCleanRequiresExactWorkspaceAndClass(t *testing.T) {
	action := workspaceclean.New(&cleaner{})
	for _, input := range []workspaceclean.Input{{}, {Workspace: "dev"}, {Workspace: "dev", Class: "artifacts"}} {
		if _, err := action.Execute(context.Background(), input); err == nil {
			t.Fatalf("input %#v succeeded", input)
		}
	}
}

func TestCleanDelegatesOneExactDisposableClass(t *testing.T) {
	store := &cleaner{result: workspaceclean.Result{Status: "cleaned", Workspace: "dev", Class: "temporary", EntriesRemoved: 3, BytesRemoved: 42}}
	output, err := workspaceclean.New(store).Execute(context.Background(), workspaceclean.Input{Workspace: "dev", Class: "temporary"})
	if err != nil {
		t.Fatal(err)
	}
	if store.request.Workspace != "dev" || store.request.Class != "temporary" || output.Result.EntriesRemoved != 3 {
		t.Fatalf("request=%#v output=%#v", store.request, output)
	}
}

func TestCleanPreservesStructuredFailure(t *testing.T) {
	_, err := workspaceclean.New(&cleaner{err: errors.New("cleanup failed")}).Execute(context.Background(), workspaceclean.Input{Workspace: "dev", Class: "cache"})
	if err == nil {
		t.Fatal("expected cleanup failure")
	}
}

func TestOutputGolden(t *testing.T) {
	output := workspaceclean.Output{Result: workspaceclean.Result{Status: "cleaned", Workspace: "dev", Class: "temporary", EntriesRemoved: 3, BytesRemoved: 42, Removed: []string{".tadx/staging", ".tadx/tmp"}}, Help: []string{"tadx workspace status --workspace dev"}}
	for _, test := range []struct {
		name string
		full bool
	}{{name: "compact.toon"}, {name: "full.toon", full: true}} {
		var buffer bytes.Buffer
		if err := render.RenderWithOptions(&buffer, output, render.Options{Full: test.full}); err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("testdata", test.name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
			t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", test.name, want, buffer.Bytes())
		}
	}
}
