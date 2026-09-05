package delete_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ahillspace/tadx/actions/workspace/artifact/delete"
	"github.com/ahillspace/tadx/internal/output"
)

type store struct {
	artifact delete.Artifact
	deleted  bool
}

func (s *store) Resolve(context.Context, delete.Input) (delete.Artifact, error) {
	return s.artifact, nil
}
func (s *store) Delete(context.Context, delete.DeleteRequest) (delete.Artifact, error) {
	s.deleted = true
	return s.artifact, nil
}

func TestExecutePreviewsThenPerformsExactDeletion(t *testing.T) {
	dependency := &store{artifact: delete.Artifact{Kind: "workbook", LUID: "wb-1", Name: "Finance", Path: "artifacts/workbook/Finance", State: "clean", CurrentFingerprint: "sha256:abc", TreeFingerprint: "sha256:tree"}}
	action := delete.New(dependency)
	preview, err := action.Execute(context.Background(), delete.Input{Workspace: "development", Kind: "workbook", LUID: "wb-1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if dependency.deleted || preview.Result != nil {
		t.Fatal("preview mutated the artifact")
	}
	var rendered bytes.Buffer
	if err := output.Render(&rendered, preview); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rendered.Bytes(), []byte("mode: preview")) || !bytes.Contains(rendered.Bytes(), []byte("dirty: false")) {
		t.Fatalf("preview output:\n%s", rendered.String())
	}
	assertGolden(t, rendered.Bytes(), "testdata/preview.toon")
	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, preview, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, full.Bytes(), "testdata/preview_full.toon")
	result, err := action.Execute(context.Background(), delete.Input{Workspace: "development", Kind: "workbook", LUID: "wb-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !dependency.deleted || result.Result == nil || result.Result.Status != "deleted" {
		t.Fatalf("result = %#v", result)
	}
}

func assertGolden(t *testing.T, actual []byte, path string) {
	t.Helper()
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, bytes.TrimSuffix(expected, []byte("\n"))) {
		t.Fatalf("golden mismatch for %s\nexpected:\n%s\nactual:\n%s", path, expected, actual)
	}
}

func TestDirtyDeletionRequiresForce(t *testing.T) {
	dependency := &store{artifact: delete.Artifact{Kind: "workbook", LUID: "wb-1", Path: "artifacts/workbook/Finance", State: "dirty", CurrentFingerprint: "sha256:new", TreeFingerprint: "sha256:tree"}}
	action := delete.New(dependency)
	if _, err := action.Execute(context.Background(), delete.Input{Workspace: "development", Kind: "workbook", LUID: "wb-1"}, false); err == nil {
		t.Fatal("dirty deletion did not require force")
	}
	if dependency.deleted {
		t.Fatal("rejected dirty deletion mutated the artifact")
	}
	if _, err := action.Execute(context.Background(), delete.Input{Workspace: "development", Kind: "workbook", LUID: "wb-1", Force: true}, true); err != nil {
		t.Fatal(err)
	}
	if dependency.deleted {
		t.Fatal("force bypassed preview")
	}
}

func TestDeleteSurfacesBoundedCleanupWarningsWithoutChangingSuccess(t *testing.T) {
	warnings := make([]string, 25)
	for index := range warnings {
		warnings[index] = fmt.Sprintf("cleanup warning %d", index)
	}
	dependency := &store{artifact: delete.Artifact{Kind: "workbook", LUID: "wb-1", Path: "artifacts/workbook/Finance", State: "clean", CurrentFingerprint: "sha256:abc", TreeFingerprint: "sha256:tree", Warnings: warnings}}
	action := delete.New(dependency)
	preview, err := action.Execute(context.Background(), delete.Input{Workspace: "development", Kind: "workbook", LUID: "wb-1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Warnings) != 0 {
		t.Fatalf("preview warnings = %#v", preview.Warnings)
	}
	result, err := action.Execute(context.Background(), delete.Input{Workspace: "development", Kind: "workbook", LUID: "wb-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result == nil || result.Result.Status != "deleted" || len(result.Warnings) != 20 || result.WarningsOmitted != 5 {
		t.Fatalf("result output = %#v", result)
	}
	var rendered bytes.Buffer
	if err := output.Render(&rendered, result); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rendered.Bytes(), []byte("warnings_omitted: 5")) {
		t.Fatalf("compact output:\n%s", rendered.String())
	}
}
