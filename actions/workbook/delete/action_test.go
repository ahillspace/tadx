package delete_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	workbookdelete "github.com/ahillspace/tadx/actions/workbook/delete"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type resolver struct {
	results []workbookdelete.Workbook
	inputs  []identity.Selector
}

func (r *resolver) ResolveWorkbook(_ context.Context, selector identity.Selector) (workbookdelete.Workbook, error) {
	r.inputs = append(r.inputs, selector)
	if len(r.results) == 0 {
		return workbookdelete.Workbook{}, errors.New("missing workbook")
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result, nil
}

type deleter struct {
	calls []string
}

func (d *deleter) DeleteWorkbook(_ context.Context, luid string) (workbookdelete.Result, error) {
	d.calls = append(d.calls, luid)
	return workbookdelete.Result{Status: "succeeded", WorkbookLUID: luid, TableauRequestID: "request-1"}, nil
}

func TestDeletePreviewsWithoutMutation(t *testing.T) {
	target := workbookdelete.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Ops"}
	r := &resolver{results: []workbookdelete.Workbook{target}}
	d := &deleter{}
	output, err := workbookdelete.New(r, d).Execute(context.Background(), workbookdelete.Input{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "wb-1"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if output.Applied || len(d.calls) != 0 || len(r.inputs) != 1 {
		t.Fatalf("output=%#v delete_calls=%v resolve_calls=%v", output, d.calls, r.inputs)
	}
}

func TestDeleteRevalidatesAuthoritativeLUIDAndAllowsMetadataChange(t *testing.T) {
	planned := workbookdelete.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Ops"}
	current := workbookdelete.Workbook{LUID: "wb-1", Name: "Finance Renamed", ProjectLUID: "project-2", ProjectPath: "Archive"}
	r := &resolver{results: []workbookdelete.Workbook{planned, current}}
	d := &deleter{}
	output, err := workbookdelete.New(r, d).Execute(context.Background(), workbookdelete.Input{Environment: "dev", Site: "sandbox", Selector: identity.Selector{Name: "Finance", ProjectPath: "Ops"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !output.Applied || len(d.calls) != 1 || d.calls[0] != "wb-1" {
		t.Fatalf("output=%#v delete_calls=%v", output, d.calls)
	}
	if len(r.inputs) != 2 || r.inputs[1].LUID != "wb-1" || r.inputs[1].Name != "" || r.inputs[1].ProjectPath != "" {
		t.Fatalf("revalidation selectors=%#v", r.inputs)
	}
}

func TestDeleteRejectsAuthoritativeIdentityChange(t *testing.T) {
	r := &resolver{results: []workbookdelete.Workbook{{LUID: "wb-1", Name: "Finance"}, {LUID: "wb-2", Name: "Finance"}}}
	d := &deleter{}
	_, err := workbookdelete.New(r, d).Execute(context.Background(), workbookdelete.Input{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "wb-1"}}, true)
	if err == nil || len(d.calls) != 0 {
		t.Fatalf("err=%v delete_calls=%v", err, d.calls)
	}
}

func TestDeleteRequiresExplicitEnvironmentAndSite(t *testing.T) {
	_, err := workbookdelete.New(&resolver{}, &deleter{}).Execute(context.Background(), workbookdelete.Input{}, false)
	if err == nil {
		t.Fatal("expected explicit target error")
	}
}

func TestOutputGolden(t *testing.T) {
	output := workbookdelete.Output{Plan: workbookdelete.Plan{Mode: "preview", Operation: "workbook.delete", Environment: "dev", Site: "sandbox", Target: workbookdelete.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Ops"}}, Applied: true, Result: &workbookdelete.Result{Status: "succeeded", WorkbookLUID: "wb-1", TableauRequestID: "request-1"}, Help: []string{"tadx content workbook list --environment dev"}}
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
