package project_test

import (
	"bytes"
	"context"
	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type inspectResolver struct {
	project projectops.InspectProject
	input   identity.Selector
}

func TestInspectOutputGolden(t *testing.T) {
	workbookCount := 2
	item := projectops.InspectProject{
		LUID: "project-1", Name: "Ops", Path: "Department/Ops", ParentLUID: "project-root",
		Description: "Operations", OwnerLUID: "user-1", WorkbookCount: &workbookCount, RequestID: "request-1",
	}
	output, err := newTestService(projectops.Ports{InspectResolver: &inspectResolver{project: item}}).InspectProject(t.Context(), projectops.InspectInput{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if output.Source == nil || output.Source.Mode != "tableau" {
		t.Fatalf("live entry point lost source evidence: %+v", output.Source)
	}
	// The historical golden describes the project projection without source metadata.
	output.Source = nil
	inspectAssertGolden(t, "compact.toon", output, false)
	inspectAssertGolden(t, "full.toon", output, true)
}

func inspectAssertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata/inspect", name))
	if err != nil {
		t.Fatal(err)
	}
	gotText := strings.ReplaceAll(buffer.String(), "\r\n", "\n")
	wantText := strings.ReplaceAll(string(want), "\r\n", "\n")
	gotText = strings.TrimSuffix(gotText, "\n")
	wantText = strings.TrimSuffix(wantText, "\n")
	if gotText != wantText {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, wantText, gotText)
	}
}

func (r *inspectResolver) ResolveProject(_ context.Context, selector identity.Selector) (projectops.InspectProject, error) {
	r.input = selector
	return r.project, nil
}

func TestInspectActionGetsExactProject(t *testing.T) {
	r := &inspectResolver{project: projectops.InspectProject{LUID: "p-2", Name: "Ops", Path: "Department/Ops", ParentLUID: "p-1", Description: "Operations"}}
	output, err := newTestService(projectops.Ports{InspectResolver: r}).InspectProject(context.Background(), projectops.InspectInput{Environment: "dev", Site: "site", Selector: identity.Selector{ProjectPath: "Department/Ops"}})
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(projectops.InspectCompactResult)
	if compact.Project.Path != "Department/Ops" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
	if output.FullOutput().(projectops.InspectFullResult).Project.Description != "Operations" {
		t.Fatalf("full = %#v", output.FullOutput())
	}
}

func TestInspectActionRequiresExactSelector(t *testing.T) {
	_, err := newTestService(projectops.Ports{InspectResolver: &inspectResolver{}}).InspectProject(context.Background(), projectops.InspectInput{})
	if err == nil {
		t.Fatal("expected selector error")
	}
}
