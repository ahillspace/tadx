package inspect_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	projectget "github.com/ahillspace/tadx/actions/project/inspect"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type resolver struct {
	project projectget.Project
	input   identity.Selector
}

func TestOutputGolden(t *testing.T) {
	workbookCount := 2
	item := projectget.Project{
		LUID: "project-1", Name: "Ops", Path: "Department/Ops", ParentLUID: "project-root",
		Description: "Operations", OwnerLUID: "user-1", WorkbookCount: &workbookCount, RequestID: "request-1",
	}
	output, err := projectget.New(&resolver{project: item}).Execute(t.Context(), projectget.Input{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}})
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
}

func assertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", name))
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

func (r *resolver) ResolveProject(_ context.Context, selector identity.Selector) (projectget.Project, error) {
	r.input = selector
	return r.project, nil
}

func TestActionGetsExactProject(t *testing.T) {
	r := &resolver{project: projectget.Project{LUID: "p-2", Name: "Ops", Path: "Department/Ops", ParentLUID: "p-1", Description: "Operations"}}
	output, err := projectget.New(r).Execute(context.Background(), projectget.Input{Environment: "dev", Site: "site", Selector: identity.Selector{ProjectPath: "Department/Ops"}})
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(projectget.CompactResult)
	if compact.Project.Path != "Department/Ops" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
	if output.FullOutput().(projectget.FullResult).Project.Description != "Operations" {
		t.Fatalf("full = %#v", output.FullOutput())
	}
}

func TestActionRequiresExactSelector(t *testing.T) {
	_, err := projectget.New(&resolver{}).Execute(context.Background(), projectget.Input{})
	if err == nil {
		t.Fatal("expected selector error")
	}
}
