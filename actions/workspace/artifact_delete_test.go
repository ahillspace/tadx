package workspace_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	"github.com/ahillspace/tadx/internal/output"
)

type deleteArtifactStore struct {
	artifact workspaceaction.ArtifactTarget
	deleted  bool
}

func (s *deleteArtifactStore) ResolveArtifact(context.Context, workspaceaction.DeleteArtifactInput) (workspaceaction.ArtifactTarget, error) {
	return s.artifact, nil
}
func (s *deleteArtifactStore) DeleteArtifact(context.Context, workspaceaction.ArtifactDeleteRequest) (workspaceaction.ArtifactTarget, error) {
	s.deleted = true
	return s.artifact, nil
}

func TestDeleteArtifactExecutePreviewsThenPerformsExactDeletion(t *testing.T) {
	dependency := &deleteArtifactStore{artifact: workspaceaction.ArtifactTarget{Kind: "workbook", LUID: "wb-1", Name: "Finance", Path: "artifacts/workbook/Finance", State: "clean", CurrentFingerprint: "sha256:abc", TreeFingerprint: "sha256:tree"}}
	action := (&workspaceaction.Service{ArtifactStore: dependency})
	preview, err := action.DeleteArtifact(t.Context(), workspaceaction.DeleteArtifactInput{Workspace: "development", Kind: "workbook", LUID: "wb-1"}, true)
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
	assertGolden(t, rendered.Bytes(), "testdata/artifact_delete/preview.toon")
	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, preview, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, full.Bytes(), "testdata/artifact_delete/preview_full.toon")
	result, err := action.DeleteArtifact(t.Context(), workspaceaction.DeleteArtifactInput{Workspace: "development", Kind: "workbook", LUID: "wb-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !dependency.deleted || result.Result == nil || result.Result.Status != "deleted" {
		t.Fatalf("result = %#v", result)
	}
}

func TestDeleteArtifactDirtyDeletionRequiresForce(t *testing.T) {
	dependency := &deleteArtifactStore{artifact: workspaceaction.ArtifactTarget{Kind: "workbook", LUID: "wb-1", Path: "artifacts/workbook/Finance", State: "dirty", CurrentFingerprint: "sha256:new", TreeFingerprint: "sha256:tree"}}
	action := (&workspaceaction.Service{ArtifactStore: dependency})
	if _, err := action.DeleteArtifact(t.Context(), workspaceaction.DeleteArtifactInput{Workspace: "development", Kind: "workbook", LUID: "wb-1"}, false); err == nil {
		t.Fatal("dirty deletion did not require force")
	}
	if dependency.deleted {
		t.Fatal("rejected dirty deletion mutated the artifact")
	}
	if _, err := action.DeleteArtifact(t.Context(), workspaceaction.DeleteArtifactInput{Workspace: "development", Kind: "workbook", LUID: "wb-1", Force: true}, true); err != nil {
		t.Fatal(err)
	}
	if dependency.deleted {
		t.Fatal("force bypassed preview")
	}
}

func TestDeleteArtifactDeleteSurfacesBoundedCleanupWarningsWithoutChangingSuccess(t *testing.T) {
	warnings := make([]string, 25)
	for index := range warnings {
		warnings[index] = fmt.Sprintf("cleanup warning %d", index)
	}
	dependency := &deleteArtifactStore{artifact: workspaceaction.ArtifactTarget{Kind: "workbook", LUID: "wb-1", Path: "artifacts/workbook/Finance", State: "clean", CurrentFingerprint: "sha256:abc", TreeFingerprint: "sha256:tree", Warnings: warnings}}
	action := (&workspaceaction.Service{ArtifactStore: dependency})
	preview, err := action.DeleteArtifact(t.Context(), workspaceaction.DeleteArtifactInput{Workspace: "development", Kind: "workbook", LUID: "wb-1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Warnings) != 0 {
		t.Fatalf("preview warnings = %#v", preview.Warnings)
	}
	result, err := action.DeleteArtifact(t.Context(), workspaceaction.DeleteArtifactInput{Workspace: "development", Kind: "workbook", LUID: "wb-1"}, false)
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
