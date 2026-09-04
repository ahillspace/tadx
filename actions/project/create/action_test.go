package create_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	projectcreate "github.com/ahillspace/tadx/actions/project/create"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type resolver struct {
	parents    []projectcreate.Project
	collisions []projectcreate.Project
	parentCall int
}

func (r *resolver) ResolveProject(context.Context, identity.Selector) (projectcreate.Project, error) {
	if len(r.parents) == 0 {
		return projectcreate.Project{}, errors.New("missing parent")
	}
	index := r.parentCall
	if index >= len(r.parents) {
		index = len(r.parents) - 1
	}
	r.parentCall++
	return r.parents[index], nil
}

func (r *resolver) FindProjectCollisions(context.Context, string, string) ([]projectcreate.Project, error) {
	return append([]projectcreate.Project(nil), r.collisions...), nil
}

type creator struct {
	calls int
	input projectcreate.CreateRequest
}

func TestOutputGolden(t *testing.T) {
	output := projectcreate.Output{
		Plan: projectcreate.Plan{
			Mode: "preview", Operation: "project.create", Environment: "dev", Site: "sandbox",
			Project: projectcreate.ProjectSpec{Name: "Operations", Description: "Direct operations", ContentPermissions: "LockedToProject"},
			Parent:  &projectcreate.Project{LUID: "parent-1", Name: "Department", Path: "Department"},
		},
		Applied: true,
		Result:  &projectcreate.Result{Status: "succeeded", Project: projectcreate.Project{LUID: "project-1", Name: "Operations", Path: "Department/Operations", ParentLUID: "parent-1", Description: "Direct operations", ContentPermissions: "LockedToProject"}, TableauRequestID: "request-1"},
		Help:    []string{"tadx content project get --project-id project-1"},
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
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

func (c *creator) CreateProject(_ context.Context, input projectcreate.CreateRequest) (projectcreate.Result, error) {
	c.calls++
	c.input = input
	return projectcreate.Result{Status: "succeeded", Project: projectcreate.Project{LUID: "project-1", Name: input.Name, ParentLUID: input.ParentLUID, Path: "Department/" + input.Name}, TableauRequestID: "request-1"}, nil
}

func TestCreatePreviewsThenRevalidatesParentAndCollisionOnApply(t *testing.T) {
	r := &resolver{parents: []projectcreate.Project{{LUID: "parent-1", Name: "Department", Path: "Department"}, {LUID: "parent-1", Name: "Renamed", Path: "Renamed"}}}
	c := &creator{}
	action := projectcreate.New(r, c)
	input := projectcreate.Input{Environment: "dev", Site: "sandbox", Name: "Operations", Description: "Direct operations", ContentPermissions: "LockedToProject", ParentSelector: identity.Selector{LUID: "parent-1"}}

	preview, err := action.Execute(context.Background(), input, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || c.calls != 0 || preview.Plan.Parent == nil || preview.Plan.Parent.LUID != "parent-1" {
		t.Fatalf("preview=%#v creator=%#v", preview, c)
	}

	applied, err := action.Execute(context.Background(), input, true)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || c.calls != 1 || c.input.ParentLUID != "parent-1" || applied.Result == nil || applied.Result.Project.LUID != "project-1" {
		t.Fatalf("applied=%#v creator=%#v", applied, c)
	}
}

func TestCreateRootDoesNotResolveOrInferParent(t *testing.T) {
	r := &resolver{}
	c := &creator{}
	output, err := projectcreate.New(r, c).Execute(context.Background(), projectcreate.Input{Environment: "dev", Site: "sandbox", Name: "Root"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if r.parentCall != 0 || c.input.ParentLUID != "" || output.Plan.Parent != nil {
		t.Fatalf("output=%#v resolver=%#v creator=%#v", output, r, c)
	}
}

func TestCreateRejectsCollisionAndChangedParentIdentity(t *testing.T) {
	t.Run("collision", func(t *testing.T) {
		r := &resolver{collisions: []projectcreate.Project{{LUID: "existing", Name: "Operations"}}}
		c := &creator{}
		_, err := projectcreate.New(r, c).Execute(context.Background(), projectcreate.Input{Environment: "dev", Site: "sandbox", Name: "operations"}, false)
		if err == nil || c.calls != 0 {
			t.Fatalf("error=%v calls=%d", err, c.calls)
		}
	})
	t.Run("parent identity changed", func(t *testing.T) {
		r := &resolver{parents: []projectcreate.Project{{LUID: "parent-1"}, {LUID: "parent-2"}}}
		c := &creator{}
		_, err := projectcreate.New(r, c).Execute(context.Background(), projectcreate.Input{Environment: "dev", Site: "sandbox", Name: "Operations", ParentSelector: identity.Selector{LUID: "parent-1"}}, true)
		if err == nil || c.calls != 0 {
			t.Fatalf("error=%v calls=%d", err, c.calls)
		}
	})
}

func TestCreateRequiresExplicitTargetAndValidFields(t *testing.T) {
	tests := []projectcreate.Input{
		{Name: "Operations"},
		{Environment: "dev", Site: "sandbox"},
		{Environment: "dev", Site: "sandbox", Name: "Operations", ContentPermissions: "invalid"},
		{Environment: "dev", Site: "sandbox", Name: "Operations/Reports"},
		{Environment: "dev", Site: "sandbox", Name: "Operations", ParentSelector: identity.Selector{LUID: "parent-1", ProjectPath: "Department"}},
	}
	for _, input := range tests {
		if _, err := projectcreate.New(&resolver{}, &creator{}).Execute(context.Background(), input, false); err == nil {
			t.Fatalf("input accepted: %#v", input)
		}
	}
}

func TestCreateCompactOutputOmitsSuccessfulRequestID(t *testing.T) {
	output := projectcreate.Output{Result: &projectcreate.Result{Status: "succeeded", Project: projectcreate.Project{LUID: "project-1", Name: "Operations"}, TableauRequestID: "request-secret"}}
	compact := output.CompactOutput().(projectcreate.CompactResult)
	if compact.Result == nil || compact.Result.Project.LUID != "project-1" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
}
