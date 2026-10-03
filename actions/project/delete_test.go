package project_test

import (
	"context"
	"errors"
	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"testing"
)

type deleteResolver struct {
	results []projectops.DeleteProject
	inputs  []identity.Selector
}

func (r *deleteResolver) ResolveProject(_ context.Context, selector identity.Selector) (projectops.DeleteProject, error) {
	r.inputs = append(r.inputs, selector)
	if len(r.results) == 0 {
		return projectops.DeleteProject{}, errors.New("missing project")
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result, nil
}

type deleter struct{ calls []string }

func (d *deleter) DeleteProject(_ context.Context, luid string) (projectops.DeleteResult, error) {
	d.calls = append(d.calls, luid)
	return projectops.DeleteResult{Status: "succeeded", ProjectLUID: luid, TableauRequestID: "request-1"}, nil
}

func TestDeletePreviewsWithoutMutation(t *testing.T) {
	r := &deleteResolver{results: []projectops.DeleteProject{{LUID: "project-1", Name: "Operations", Path: "Department/Operations"}}}
	d := &deleter{}
	output, err := projectops.New(projectops.Ports{DeleteResolver: r, Deleter: d}).Delete(context.Background(), projectops.DeleteInput{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result != nil || len(d.calls) != 0 || len(r.inputs) != 1 {
		t.Fatalf("output=%#v delete_calls=%v resolve_calls=%v", output, d.calls, r.inputs)
	}
	if len(output.Warnings) != 1 || output.Warnings[0] != projectops.DeleteCascadeWarning {
		t.Fatalf("warnings=%v", output.Warnings)
	}
}

func TestDeleteRevalidatesExactLUID(t *testing.T) {
	planned := projectops.DeleteProject{LUID: "project-1", Name: "Operations", Path: "Department/Operations"}
	current := projectops.DeleteProject{LUID: "project-1", Name: "Operations renamed", Path: "Archive/Operations renamed"}
	r := &deleteResolver{results: []projectops.DeleteProject{planned, current}}
	d := &deleter{}
	output, err := projectops.New(projectops.Ports{DeleteResolver: r, Deleter: d}).Delete(context.Background(), projectops.DeleteInput{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1"}, false)
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
	r := &deleteResolver{}
	d := &deleter{}
	_, err := projectops.New(projectops.Ports{DeleteResolver: r, Deleter: d}).Delete(context.Background(), projectops.DeleteInput{Environment: "dev", Site: "sandbox"}, false)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "project.delete.usage" || structured.Kind != errs.KindUsage {
		t.Fatalf("error=%#v", err)
	}
	if len(r.inputs) != 0 || len(d.calls) != 0 {
		t.Fatalf("resolution=%v deletion=%v", r.inputs, d.calls)
	}
}

func TestDeleteRejectsIdentityChange(t *testing.T) {
	r := &deleteResolver{results: []projectops.DeleteProject{{LUID: "project-1", Name: "Operations"}, {LUID: "project-2", Name: "Operations"}}}
	d := &deleter{}
	_, err := projectops.New(projectops.Ports{DeleteResolver: r, Deleter: d}).Delete(context.Background(), projectops.DeleteInput{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1"}, false)
	if err == nil || len(d.calls) != 0 {
		t.Fatalf("err=%v deletion=%v", err, d.calls)
	}
}

func TestDeleteRejectsResolutionThatChangesRequestedLUID(t *testing.T) {
	r := &deleteResolver{results: []projectops.DeleteProject{{LUID: "project-2", Name: "Operations"}}}
	d := &deleter{}
	_, err := projectops.New(projectops.Ports{DeleteResolver: r, Deleter: d}).Delete(context.Background(), projectops.DeleteInput{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1"}, true)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "project.delete.resolve" || len(d.calls) != 0 {
		t.Fatalf("err=%v deletion=%v", err, d.calls)
	}
}
