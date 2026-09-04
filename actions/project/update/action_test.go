package update_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	projectupdate "github.com/ahillspace/tadx/actions/project/update"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type resolver struct {
	projects []projectupdate.Project
	calls    int
}

func (r *resolver) ResolveProject(context.Context, identity.Selector) (projectupdate.Project, error) {
	index := r.calls
	if index >= len(r.projects) {
		index = len(r.projects) - 1
	}
	r.calls++
	return r.projects[index], nil
}

type updater struct {
	calls int
	input projectupdate.UpdateRequest
}

func TestOutputGolden(t *testing.T) {
	name := "Renamed"
	permissions := "ManagedByOwner"
	output := projectupdate.Output{
		Plan: projectupdate.Plan{
			Mode: "preview", Operation: "project.update", Environment: "dev", Site: "sandbox",
			Target:  projectupdate.Project{LUID: "project-1", Name: "Operations", Path: "Department/Operations", ParentLUID: "parent-1", Description: "Old", ContentPermissions: "LockedToProject"},
			Changes: projectupdate.Changes{Name: &name, ContentPermissions: &permissions},
		},
		Applied: true,
		Result:  &projectupdate.Result{Status: "succeeded", Project: projectupdate.Project{LUID: "project-1", Name: "Renamed", Path: "Department/Renamed", ParentLUID: "parent-1", Description: "Old", ContentPermissions: "ManagedByOwner"}, TableauRequestID: "request-1"},
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

func (u *updater) UpdateProject(_ context.Context, input projectupdate.UpdateRequest) (projectupdate.Result, error) {
	u.calls++
	u.input = input
	project := projectupdate.Project{LUID: input.LUID, Name: "Renamed", Path: "Department/Renamed", ParentLUID: "parent-1", Description: "Current", ContentPermissions: "ManagedByOwner"}
	return projectupdate.Result{Status: "succeeded", Project: project, TableauRequestID: "request-1"}, nil
}

func stringPointer(value string) *string { return &value }

func TestUpdatePreviewsAndAppliesOnlyChangedFields(t *testing.T) {
	r := &resolver{projects: []projectupdate.Project{
		{LUID: "project-1", Name: "Operations", Path: "Department/Operations", ParentLUID: "parent-1", Description: "Old", ContentPermissions: "LockedToProject"},
		{LUID: "project-1", Name: "Operations", Path: "RenamedParent/Operations", ParentLUID: "parent-1", Description: "Current", ContentPermissions: "LockedToProject"},
	}}
	u := &updater{}
	action := projectupdate.New(r, u)
	input := projectupdate.Input{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}, Name: stringPointer("Renamed"), Description: stringPointer("Current"), ContentPermissions: stringPointer("ManagedByOwner")}

	preview, err := action.Execute(context.Background(), input, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || preview.Plan.NoOp || u.calls != 0 {
		t.Fatalf("preview=%#v updater=%#v", preview, u)
	}

	applied, err := action.Execute(context.Background(), input, true)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || u.calls != 1 || u.input.LUID != "project-1" || u.input.Name == nil || *u.input.Name != "Renamed" || u.input.Description != nil || u.input.ContentPermissions == nil {
		t.Fatalf("applied=%#v updater=%#v", applied, u)
	}
}

func TestUpdateEqualValuesAreNoOpWithoutPut(t *testing.T) {
	project := projectupdate.Project{LUID: "project-1", Name: "Operations", Path: "Operations", Description: "Same", ContentPermissions: "ManagedByOwner"}
	r := &resolver{projects: []projectupdate.Project{project}}
	u := &updater{}
	output, err := projectupdate.New(r, u).Execute(context.Background(), projectupdate.Input{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}, Description: stringPointer("Same")}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !output.Applied || !output.Plan.NoOp || output.Result == nil || output.Result.Status != "unchanged" || u.calls != 0 {
		t.Fatalf("output=%#v updater=%#v", output, u)
	}
}

func TestUpdateRejectsChangedTargetIdentity(t *testing.T) {
	r := &resolver{projects: []projectupdate.Project{{LUID: "project-1", Name: "Operations"}, {LUID: "project-2", Name: "Operations"}}}
	u := &updater{}
	_, err := projectupdate.New(r, u).Execute(context.Background(), projectupdate.Input{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}, Name: stringPointer("Renamed")}, true)
	if err == nil || u.calls != 0 {
		t.Fatalf("error=%v calls=%d", err, u.calls)
	}
}

func TestUpdateRequiresExplicitTargetSelectorAndChanges(t *testing.T) {
	project := projectupdate.Project{LUID: "project-1", Name: "Operations"}
	tests := []projectupdate.Input{
		{Selector: identity.Selector{LUID: "project-1"}, Name: stringPointer("Renamed")},
		{Environment: "dev", Site: "sandbox", Name: stringPointer("Renamed")},
		{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}},
		{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}, ContentPermissions: stringPointer("invalid")},
		{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}, Name: stringPointer("Operations/Reports")},
		{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1", ProjectPath: "Department/Operations"}, Name: stringPointer("Renamed")},
	}
	for _, input := range tests {
		if _, err := projectupdate.New(&resolver{projects: []projectupdate.Project{project}}, &updater{}).Execute(context.Background(), input, false); err == nil {
			t.Fatalf("input accepted: %#v", input)
		}
	}
}

func TestUpdateCompactOutputOmitsSuccessfulRequestID(t *testing.T) {
	output := projectupdate.Output{Result: &projectupdate.Result{Status: "succeeded", Project: projectupdate.Project{LUID: "project-1", Name: "Operations"}, TableauRequestID: "request-secret"}}
	compact := output.CompactOutput().(projectupdate.CompactResult)
	if compact.Result == nil || compact.Result.Project.LUID != "project-1" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
}
