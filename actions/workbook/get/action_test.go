package get_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	workbookget "github.com/ahillspace/tadx/actions/workbook/get"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type resolver struct {
	workbook workbookget.Workbook
	selector identity.Selector
}

func (r *resolver) ResolveWorkbook(_ context.Context, selector identity.Selector) (workbookget.Workbook, error) {
	r.selector = selector
	return r.workbook, nil
}

func TestOutputGolden(t *testing.T) {
	output := workbookget.Output{
		Status: "found", Environment: "dev", Site: "sandbox", RequestID: "request-1",
		Workbook: workbookget.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Department/Ops", ContentURL: "Finance", UpdatedAt: "2026-09-01T00:00:00Z", Description: "Finance reporting", OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z", Tags: []string{"finance"}},
		Help:     []string{"tadx content workbook pull --id wb-1"},
	}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
}

func TestActionGetsExactWorkbookAndBoundsTags(t *testing.T) {
	tags := make([]string, 60)
	for index := range tags {
		tags[index] = fmt.Sprintf("tag-%02d", index)
	}
	r := &resolver{workbook: workbookget.Workbook{LUID: "wb-1", Name: "Finance", ProjectPath: "Department/Ops", Tags: tags, RequestID: "request-1"}}
	output, err := workbookget.New(r).Execute(context.Background(), workbookget.Input{Environment: "dev", Selector: identity.Selector{Name: "Finance", ProjectPath: "Department/Ops"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.selector.Name != "Finance" || r.selector.ProjectPath != "Department/Ops" {
		t.Fatalf("selector = %#v", r.selector)
	}
	if output.RequestID != "request-1" || output.CompactOutput().(workbookget.CompactResult).Workbook.ProjectPath != "Department/Ops" {
		t.Fatalf("output = %#v", output)
	}
	full := output.FullOutput().(workbookget.FullResult)
	if len(full.Workbook.Tags) != 50 || full.Workbook.TagsOmitted != 10 {
		t.Fatalf("full = %#v", full)
	}
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
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}
