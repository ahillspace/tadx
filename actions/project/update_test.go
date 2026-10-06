package project_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type updateResolver struct {
	projects []projectops.UpdateProject
	calls    int
}

func (r *updateResolver) ResolveProject(context.Context, identity.Selector) (projectops.UpdateProject, error) {
	index := r.calls
	if index >= len(r.projects) {
		index = len(r.projects) - 1
	}
	r.calls++
	return r.projects[index], nil
}

type updater struct {
	calls int
	input projectops.UpdateRequest
}

func TestProjectUpdateOutputGolden(t *testing.T) {
	name := "Renamed"
	permissions := "ManagedByOwner"
	output := projectops.UpdateOutput{
		Plan: projectops.UpdatePlan{
			Mode: "execute", Operation: "project.update", Environment: "dev", Site: "sandbox",
			Target:  projectops.UpdateProject{LUID: "project-1", Name: "Operations", Path: "Department/Operations", ParentLUID: "parent-1", Description: "Old", ContentPermissions: "LockedToProject"},
			Changes: projectops.Changes{Name: &name, ContentPermissions: &permissions},
		},
		Result: &projectops.UpdateResult{Status: "succeeded", Project: projectops.UpdateProject{LUID: "project-1", Name: "Renamed", Path: "Department/Renamed", ParentLUID: "parent-1", Description: "Old", ContentPermissions: "ManagedByOwner"}, TableauRequestID: "request-1"},
		Help:   []string{"tadx content project inspect --project-id project-1"},
	}
	assertUpdateGolden(t, "compact.toon", output, false)
	assertUpdateGolden(t, "full.toon", output, true)
}

func assertUpdateGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata/update", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

func (u *updater) UpdateProject(_ context.Context, input projectops.UpdateRequest) (projectops.UpdateResult, error) {
	u.calls++
	u.input = input
	project := projectops.UpdateProject{LUID: input.LUID, Name: "Renamed", Path: "Department/Renamed", ParentLUID: "parent-1", Description: "Current", ContentPermissions: "ManagedByOwner"}
	return projectops.UpdateResult{Status: "succeeded", Project: project, TableauRequestID: "request-1"}, nil
}

func stringPointer(value string) *string { return &value }

func TestUpdatePreviewsAndAppliesOnlyChangedFields(t *testing.T) {
	r := &updateResolver{projects: []projectops.UpdateProject{
		{LUID: "project-1", Name: "Operations", Path: "Department/Operations", ParentLUID: "parent-1", Description: "Old", ContentPermissions: "LockedToProject"},
		{LUID: "project-1", Name: "Operations", Path: "RenamedParent/Operations", ParentLUID: "parent-1", Description: "Current", ContentPermissions: "LockedToProject"},
	}}
	u := &updater{}
	action := projectops.NewUpdate(r, u)
	input := projectops.UpdateInput{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}, Name: stringPointer("Renamed"), Description: stringPointer("Current"), ContentPermissions: stringPointer("ManagedByOwner")}

	preview, err := action.Execute(context.Background(), input, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Result != nil || preview.Plan.NoOp || u.calls != 0 {
		t.Fatalf("preview=%#v updater=%#v", preview, u)
	}

	result, err := action.Execute(context.Background(), input, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result == nil || u.calls != 1 || u.input.LUID != "project-1" || u.input.Name == nil || *u.input.Name != "Renamed" || u.input.Description != nil || u.input.ContentPermissions == nil {
		t.Fatalf("result=%#v updater=%#v", result, u)
	}
}

func TestUpdateEqualValuesAreNoOpWithoutPut(t *testing.T) {
	project := projectops.UpdateProject{LUID: "project-1", Name: "Operations", Path: "Operations", Description: "Same", ContentPermissions: "ManagedByOwner"}
	r := &updateResolver{projects: []projectops.UpdateProject{project}}
	u := &updater{}
	output, err := projectops.NewUpdate(r, u).Execute(context.Background(), projectops.UpdateInput{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}, Description: stringPointer("Same")}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !output.Plan.NoOp || output.Result == nil || output.Result.Status != "unchanged" || u.calls != 0 {
		t.Fatalf("output=%#v updater=%#v", output, u)
	}
}

func TestUpdateRejectsChangedTargetIdentity(t *testing.T) {
	r := &updateResolver{projects: []projectops.UpdateProject{{LUID: "project-1", Name: "Operations"}, {LUID: "project-2", Name: "Operations"}}}
	u := &updater{}
	_, err := projectops.NewUpdate(r, u).Execute(context.Background(), projectops.UpdateInput{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}, Name: stringPointer("Renamed")}, false)
	if err == nil || u.calls != 0 {
		t.Fatalf("error=%v calls=%d", err, u.calls)
	}
}

func TestUpdateRequiresExplicitTargetSelectorAndChanges(t *testing.T) {
	project := projectops.UpdateProject{LUID: "project-1", Name: "Operations"}
	tests := []projectops.UpdateInput{
		{Selector: identity.Selector{LUID: "project-1"}, Name: stringPointer("Renamed")},
		{Environment: "dev", Site: "sandbox", Name: stringPointer("Renamed")},
		{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}},
		{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}, ContentPermissions: stringPointer("invalid")},
		{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1"}, Name: stringPointer("Operations/Reports")},
		{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "project-1", ProjectPath: "Department/Operations"}, Name: stringPointer("Renamed")},
	}
	for _, input := range tests {
		if _, err := projectops.NewUpdate(&updateResolver{projects: []projectops.UpdateProject{project}}, &updater{}).Execute(context.Background(), input, false); err == nil {
			t.Fatalf("input accepted: %#v", input)
		}
	}
}

func TestUpdateCompactOutputOmitsSuccessfulRequestID(t *testing.T) {
	output := projectops.UpdateOutput{Result: &projectops.UpdateResult{Status: "succeeded", Project: projectops.UpdateProject{LUID: "project-1", Name: "Operations"}, TableauRequestID: "request-secret"}}
	compact := output.CompactOutput().(projectops.UpdateCompactResult)
	if compact.Result == nil || compact.Result.Project.LUID != "project-1" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
}
