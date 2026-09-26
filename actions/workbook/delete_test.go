package workbook_test

import (
	"bytes"
	"context"
	"errors"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/value"
	"os"
	"path/filepath"
	"testing"
)

type deleteResolver struct {
	results []workbookops.Record
	inputs  []identity.Selector
}

func (r *deleteResolver) ResolveWorkbook(_ context.Context, selector identity.Selector) (workbookops.Record, error) {
	r.inputs = append(r.inputs, selector)
	if len(r.results) == 0 {
		return workbookops.Record{}, errors.New("missing workbook")
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result, nil
}

type deleteDeleter struct {
	calls []string
}

func (d *deleteDeleter) DeleteWorkbook(_ context.Context, luid string) (workbookops.DeleteResult, error) {
	d.calls = append(d.calls, luid)
	return workbookops.DeleteResult{Status: "succeeded", WorkbookLUID: luid, TableauRequestID: "request-1"}, nil
}

func TestDeleteDeletePreviewsWithoutMutation(t *testing.T) {
	target := workbookops.Record{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Ops"}
	r := &deleteResolver{results: []workbookops.Record{target}}
	d := &deleteDeleter{}
	output, err := workbookops.Delete(context.Background(), r, d, workbookops.DeleteInput{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "wb-1"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result != nil || len(d.calls) != 0 || len(r.inputs) != 1 {
		t.Fatalf("output=%#v delete_calls=%v resolve_calls=%v", output, d.calls, r.inputs)
	}
}

func TestDeleteDeleteRevalidatesAuthoritativeLUIDAndAllowsMetadataChange(t *testing.T) {
	planned := workbookops.Record{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Ops"}
	current := workbookops.Record{LUID: "wb-1", Name: "Finance Renamed", ProjectLUID: "project-2", ProjectPath: "Archive"}
	r := &deleteResolver{results: []workbookops.Record{planned, current}}
	d := &deleteDeleter{}
	output, err := workbookops.Delete(context.Background(), r, d, workbookops.DeleteInput{Environment: "dev", Site: "sandbox", Selector: identity.Selector{Name: "Finance", ProjectPath: "Ops"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result == nil || len(d.calls) != 1 || d.calls[0] != "wb-1" {
		t.Fatalf("output=%#v delete_calls=%v", output, d.calls)
	}
	if len(r.inputs) != 2 || r.inputs[1].LUID != "wb-1" || r.inputs[1].Name != "" || r.inputs[1].ProjectPath != "" {
		t.Fatalf("revalidation selectors=%#v", r.inputs)
	}
}

func TestDeleteDeleteRejectsAuthoritativeIdentityChange(t *testing.T) {
	r := &deleteResolver{results: []workbookops.Record{{LUID: "wb-1", Name: "Finance"}, {LUID: "wb-2", Name: "Finance"}}}
	d := &deleteDeleter{}
	_, err := workbookops.Delete(context.Background(), r, d, workbookops.DeleteInput{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "wb-1"}}, false)
	if err == nil || len(d.calls) != 0 {
		t.Fatalf("err=%v delete_calls=%v", err, d.calls)
	}
}

func TestDeleteDeleteRequiresExplicitEnvironmentAndSite(t *testing.T) {
	_, err := workbookops.Delete(context.Background(), &deleteResolver{}, &deleteDeleter{}, workbookops.DeleteInput{}, false)
	if err == nil {
		t.Fatal("expected explicit target error")
	}
}

func TestDeleteDeleteRejectsInvalidSelectorAtSeam(t *testing.T) {
	cases := []struct {
		name     string
		selector identity.Selector
	}{
		{"empty", identity.Selector{}},
		{"whitespace only", identity.Selector{Name: "   ", ProjectPath: "\t"}},
		{"name without project", identity.Selector{Name: "Finance"}},
		{"conflicting luid and name", identity.Selector{LUID: "wb-1", Name: "Finance", ProjectPath: "Ops"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &deleteResolver{}
			d := &deleteDeleter{}
			_, err := workbookops.Delete(context.Background(), r, d, workbookops.DeleteInput{Environment: "dev", Site: "sandbox", Selector: tc.selector}, false)
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.ID != "workbook.delete.usage" {
				t.Fatalf("error = %#v", structured)
			}
			if len(r.inputs) != 0 || len(d.calls) != 0 {
				t.Fatalf("seam validation must not reach adapters: resolve=%v delete=%v", r.inputs, d.calls)
			}
		})
	}
}

func TestDeleteOutputGolden(t *testing.T) {
	output := workbookops.DeleteOutput{Plan: workbookops.DeletePlan{Mode: "execute", Operation: "workbook.delete", Environment: "dev", Site: "sandbox", Target: value.ContentIdentity{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Ops"}}, Result: &workbookops.DeleteResult{Status: "succeeded", WorkbookLUID: "wb-1", TableauRequestID: "request-1"}, Help: []string{"tadx content workbook list --environment dev"}}
	for _, test := range []struct {
		name string
		full bool
	}{{name: "compact.toon"}, {name: "full.toon", full: true}} {
		var buffer bytes.Buffer
		if err := render.RenderWithOptions(&buffer, output, render.Options{Full: test.full}); err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("testdata/delete", test.name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
			t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", test.name, want, buffer.Bytes())
		}
	}
}
