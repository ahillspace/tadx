package delete_test

import (
	"context"
	"errors"
	"testing"

	projectdelete "github.com/ahillspace/tadx/actions/project/delete"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type resolver struct {
	results []projectdelete.Project
	inputs  []identity.Selector
}

func (r *resolver) ResolveProject(_ context.Context, selector identity.Selector) (projectdelete.Project, error) {
	r.inputs = append(r.inputs, selector)
	if len(r.results) == 0 {
		return projectdelete.Project{}, errors.New("missing project")
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result, nil
}

type deleter struct{ calls []string }

func (d *deleter) DeleteProject(_ context.Context, luid string) (projectdelete.Result, error) {
	d.calls = append(d.calls, luid)
	return projectdelete.Result{Status: "succeeded", ProjectLUID: luid, TableauRequestID: "request-1"}, nil
}

func TestDeletePreviewsWithoutMutation(t *testing.T) {
	r := &resolver{results: []projectdelete.Project{{LUID: "project-1", Name: "Operations", Path: "Department/Operations"}}}
	d := &deleter{}
	output, err := projectdelete.New(r, d).Execute(context.Background(), projectdelete.Input{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result != nil || len(d.calls) != 0 || len(r.inputs) != 1 {
		t.Fatalf("output=%#v delete_calls=%v resolve_calls=%v", output, d.calls, r.inputs)
	}
	if len(output.Warnings) != 1 || output.Warnings[0] != projectdelete.CascadeWarning {
		t.Fatalf("warnings=%v", output.Warnings)
	}
}

func TestDeleteRevalidatesExactLUID(t *testing.T) {
	planned := projectdelete.Project{LUID: "project-1", Name: "Operations", Path: "Department/Operations"}
	current := projectdelete.Project{LUID: "project-1", Name: "Operations renamed", Path: "Archive/Operations renamed"}
	r := &resolver{results: []projectdelete.Project{planned, current}}
	d := &deleter{}
	output, err := projectdelete.New(r, d).Execute(context.Background(), projectdelete.Input{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result == nil || len(d.calls) != 1 || d.calls[0] != "project-1" {
		t.Fatalf("output=%#v delete_calls=%v", output, d.calls)
	}
	if len(r.inputs) != 2 || r.inputs[0].LUID != "project-1" || r.inputs[1].LUID != "project-1" {
		t.Fatalf("revalidation selectors=%#v", r.inputs)
	}
}

func TestDeleteRejectsInvalidInputBeforeResolution(t *testing.T) {
	r := &resolver{}
	d := &deleter{}
	_, err := projectdelete.New(r, d).Execute(context.Background(), projectdelete.Input{Environment: "dev", Site: "sandbox"}, false)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "project.delete.usage" || structured.Kind != errs.KindUsage {
		t.Fatalf("error=%#v", err)
	}
	if len(r.inputs) != 0 || len(d.calls) != 0 {
		t.Fatalf("resolution=%v deletion=%v", r.inputs, d.calls)
	}
}

func TestDeleteRejectsIdentityChange(t *testing.T) {
	r := &resolver{results: []projectdelete.Project{{LUID: "project-1", Name: "Operations"}, {LUID: "project-2", Name: "Operations"}}}
	d := &deleter{}
	_, err := projectdelete.New(r, d).Execute(context.Background(), projectdelete.Input{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1"}, false)
	if err == nil || len(d.calls) != 0 {
		t.Fatalf("err=%v deletion=%v", err, d.calls)
	}
}

func TestDeleteRejectsResolutionThatChangesRequestedLUID(t *testing.T) {
	r := &resolver{results: []projectdelete.Project{{LUID: "project-2", Name: "Operations"}}}
	d := &deleter{}
	_, err := projectdelete.New(r, d).Execute(context.Background(), projectdelete.Input{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1"}, true)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "project.delete.resolve" || len(d.calls) != 0 {
		t.Fatalf("err=%v deletion=%v", err, d.calls)
	}
}
