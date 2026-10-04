package catalog

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	catalogaction "github.com/ahillspace/tadx/actions/catalog"
	"github.com/ahillspace/tadx/internal/errs"
)

type recorder struct {
	calls   int
	preview bool
	update  catalogaction.DatabaseInput
	list    catalogaction.DatabaseListInput
}

func (r *recorder) Render(any) error { return nil }
func (r *recorder) UpdateCatalogDatabase(_ context.Context, in catalogaction.DatabaseInput, p bool) (catalogaction.DatabaseOutput, error) {
	r.calls++
	r.update = in
	r.preview = p
	return catalogaction.DatabaseOutput{}, nil
}
func (r *recorder) ListCatalogDatabases(_ context.Context, in catalogaction.DatabaseListInput) (catalogaction.DatabaseListOutput, error) {
	r.calls++
	r.list = in
	return catalogaction.DatabaseListOutput{}, nil
}
func TestPreviewFalseAndRepeatedTags(t *testing.T) {
	r := &recorder{}
	c := New(Dependencies{DatabaseUpdater: r, Renderer: r})
	c.SetArgs([]string{"database", "update", "--id", "db", "--description", "Useful description", "--add-tag", "sales", "--add-tag", "retail", "--preview=false"})
	if e := c.Execute(); e != nil {
		t.Fatal(e)
	}
	if r.preview || r.calls != 1 || len(r.update.AddTags) != 2 || r.update.Description == nil {
		t.Fatalf("%+v", r)
	}
}
func TestInvalidLimitDoesNotReachService(t *testing.T) {
	r := &recorder{}
	c := New(Dependencies{DatabaseLister: r, Renderer: r})
	c.SetArgs([]string{"database", "list", "--limit", "-2"})
	if e := c.Execute(); e == nil || r.calls != 0 {
		t.Fatal("invalid bound reached service")
	}
}
func TestCapabilitiesPresent(t *testing.T) {
	c := New(Dependencies{})
	for _, kind := range []string{"database", "table", "column"} {
		for _, verb := range []string{"list", "inspect", "update"} {
			v, _, e := c.Find([]string{kind, verb})
			if e != nil || v.Annotations["tadx.capability"] != "catalog."+kind+"."+verb {
				t.Fatal(kind, verb, e)
			}
		}
	}
}

type captureRecorder struct {
	description, contact *string
	calls                int
}

func (r *captureRecorder) UpdateCatalogDatabase(_ context.Context, in catalogaction.DatabaseInput, _ bool) (catalogaction.DatabaseOutput, error) {
	r.description, r.contact = in.Description, in.ContactLUID
	r.calls++
	return catalogaction.DatabaseOutput{}, nil
}
func (r *captureRecorder) UpdateCatalogTable(_ context.Context, in catalogaction.TableInput, _ bool) (catalogaction.TableOutput, error) {
	r.description, r.contact = in.Description, in.ContactLUID
	r.calls++
	return catalogaction.TableOutput{}, nil
}
func (r *captureRecorder) UpdateCatalogColumn(_ context.Context, in catalogaction.ColumnInput, _ bool) (catalogaction.ColumnOutput, error) {
	r.description = in.Description
	r.calls++
	return catalogaction.ColumnOutput{}, nil
}
func TestRepeatedUpdateFlagCapture(t *testing.T) {
	for _, kind := range []string{"database", "table", "column"} {
		t.Run(kind, func(t *testing.T) {
			r := &captureRecorder{}
			c := New(Dependencies{DatabaseUpdater: r, TableUpdater: r, ColumnUpdater: r, Renderer: &recorder{}})
			c.SetOut(io.Discard)
			c.SetErr(io.Discard)
			base := []string{kind, "update", "--id", "asset", "--add-tag", "tag"}
			if kind == "column" {
				base = append(base, "--table-id", "parent")
			}
			run := func(extra ...string) {
				t.Helper()
				c.SetArgs(append(append([]string{}, base...), extra...))
				if err := c.Execute(); err != nil {
					t.Fatal(err)
				}
			}
			run()
			if r.description != nil || r.contact != nil {
				t.Fatal("omission became a supplied property")
			}
			run("--description", "first")
			first := r.description
			run("--description", "second")
			if first == nil || *first != "first" || r.description == nil || *r.description != "second" {
				t.Fatal("repeated capture lost value or changed prior input")
			}
			// Cobra preserves Changed and bound values when a constructed command is reused.
			run()
			if r.description == nil || *r.description != "second" {
				t.Fatal("reused command flag state changed")
			}
			if kind == "column" {
				run("--description=")
				if r.description == nil || *r.description != "" {
					t.Fatal("explicit empty description lost")
				}
			} else {
				run("--contact-id", "contact")
				if r.contact == nil || *r.contact != "contact" {
					t.Fatal("contact presence lost")
				}
			}
			if r.calls != 5 {
				t.Fatalf("calls=%d", r.calls)
			}
		})
	}
}
func TestUpdateValidationBeforeMissingCapability(t *testing.T) {
	for _, kind := range []string{"database", "table", "column"} {
		t.Run(kind, func(t *testing.T) {
			c := New(Dependencies{})
			c.SetOut(io.Discard)
			c.SetErr(io.Discard)
			base := []string{kind, "update", "--id", "asset"}
			if kind == "column" {
				base = append(base, "--table-id", "parent")
			}
			c.SetArgs(base)
			err := c.Execute()
			structured, ok := errors.AsType[*errs.Error](err)
			if !ok || structured.ID != "catalog."+kind+".update.usage" || strings.Contains(err.Error(), "capability is not configured") {
				t.Fatalf("validation ordering: %v", err)
			}
			c.SetArgs(append(base, "--description", "valid"))
			if err = c.Execute(); err == nil || !strings.Contains(err.Error(), "catalog capability is not configured") {
				t.Fatalf("missing capability: %v", err)
			}
		})
	}
}
