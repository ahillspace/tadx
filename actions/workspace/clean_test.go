package workspace_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	render "github.com/ahillspace/tadx/internal/output"
)

type cleanCleaner struct {
	request workspaceaction.CleanInput
	result  workspaceaction.CleanResult
	err     error
}

func (c *cleanCleaner) Clean(_ context.Context, request workspaceaction.CleanInput) (workspaceaction.CleanResult, error) {
	c.request = request
	return c.result, c.err
}

func TestCleanRequiresExactWorkspaceAndClass(t *testing.T) {
	action := (&workspaceaction.Service{Cleaner: &cleanCleaner{}})
	for _, input := range []workspaceaction.CleanInput{{}, {Workspace: "dev"}, {Workspace: "dev", Class: "artifacts"}} {
		if _, err := action.Clean(t.Context(), input); err == nil {
			t.Fatalf("input %#v succeeded", input)
		}
	}
}

func TestCleanDelegatesOneExactDisposableClass(t *testing.T) {
	store := &cleanCleaner{result: workspaceaction.CleanResult{Status: "cleaned", Workspace: "dev", Class: "temporary", EntriesRemoved: 3, BytesRemoved: 42}}
	output, err := (&workspaceaction.Service{Cleaner: store}).Clean(t.Context(), workspaceaction.CleanInput{Workspace: "dev", Class: "temporary"})
	if err != nil {
		t.Fatal(err)
	}
	if store.request.Workspace != "dev" || store.request.Class != "temporary" || output.EntriesRemoved != 3 {
		t.Fatalf("request=%#v output=%#v", store.request, output)
	}
}

func TestCleanPreservesStructuredFailure(t *testing.T) {
	_, err := (&workspaceaction.Service{Cleaner: &cleanCleaner{err: errors.New("cleanup failed")}}).Clean(t.Context(), workspaceaction.CleanInput{Workspace: "dev", Class: "cache"})
	if err == nil {
		t.Fatal("expected cleanup failure")
	}
}

func TestCleanOutputGolden(t *testing.T) {
	output := workspaceaction.CleanOutput{CleanResult: workspaceaction.CleanResult{Status: "cleaned", Workspace: "dev", Class: "temporary", EntriesRemoved: 3, BytesRemoved: 42, Removed: []string{".tadx/staging", ".tadx/tmp"}, CanonicalArtifactsPreserved: true}, Help: []string{"tadx workspace status --workspace dev"}}
	for _, test := range []struct {
		name string
		full bool
	}{{name: "compact.toon"}, {name: "full.toon", full: true}} {
		var buffer bytes.Buffer
		if err := render.RenderWithOptions(&buffer, output, render.Options{Full: test.full}); err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("testdata", "clean", test.name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
			t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", test.name, want, buffer.Bytes())
		}
	}
}
