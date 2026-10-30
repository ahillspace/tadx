package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/commitguard"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/output"
)

func TestPullDeniedInstallPreservesArtifactAndRendersOutcome(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writer := artifactWriter{workbooks: artifact.NewWorkbookManager(nil), bundles: artifact.NewWorkbookBundleManager(nil)}
	input := workbookops.PullInput{Environment: "source", Site: "site", ServerOrigin: "https://tableau.example.com", SiteLUID: "site-1", Workspace: workspace, Selector: identity.Selector{LUID: "wb-1"}, Overwrite: true}
	reader := workbookDependencyPullReader{dir: t.TempDir()}
	first, err := workbookops.Pull(t.Context(), reader, writer, input)
	if err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(workspace, filepath.FromSlash(first.Artifact.CanonicalPath))
	if err := os.WriteFile(canonical, []byte("local edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	ctx := commitguard.WithAdmission(t.Context(), func(context.Context) error { called = true; return context.Canceled })
	_, err = workbookops.Pull(ctx, reader, writer, input)
	if !called || !errors.Is(err, context.Canceled) {
		t.Fatalf("admission=%v error=%v", called, err)
	}
	after, readErr := os.ReadFile(canonical)
	if readErr != nil || !bytes.Equal(before, after) {
		t.Fatalf("artifact changed: %q -> %q, %v", before, after, readErr)
	}
	for _, json := range []bool{false, true} {
		var rendered bytes.Buffer
		if renderErr := output.RenderError(&rendered, err, output.Options{JSON: json}); renderErr != nil {
			t.Fatal(renderErr)
		}
		if !strings.Contains(rendered.String(), "setup") || !strings.Contains(rendered.String(), "not_attempted") || !strings.Contains(rendered.String(), "workbook.pull.write") {
			t.Fatalf("json=%v: %s", json, rendered.String())
		}
	}
}
