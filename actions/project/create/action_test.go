package create_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	projectcreate "github.com/ahillspace/tadx/actions/project/create"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type resolver struct {
	parents        []projectcreate.Project
	collisions     []projectcreate.Project
	parentCall     int
	lateCollisions []projectcreate.Project
	collisionCalls int
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
	r.collisionCalls++
	if r.collisionCalls > 1 && r.lateCollisions != nil {
		return r.lateCollisions, nil
	}
	return append([]projectcreate.Project(nil), r.collisions...), nil
}

func TestCreateRejectsLateSiblingCollisionBeforeWriting(t *testing.T) {
	r := &resolver{lateCollisions: []projectcreate.Project{{LUID: "other", Name: "Operations"}}}
	c := &creator{}
	_, err := projectcreate.New(r, c).Execute(t.Context(), projectcreate.Input{Environment: "dev", Site: "sandbox", Name: "Operations"}, false)
	if err == nil || !strings.Contains(err.Error(), "already") || r.collisionCalls != 2 || c.calls != 0 {
		t.Fatalf("error=%v collision_reads=%d writes=%d", err, r.collisionCalls, c.calls)
	}
}

type creator struct {
	calls int
	input projectcreate.CreateRequest
}

type failedCreator struct {
	result projectcreate.Result
	err    error
}

func (c failedCreator) CreateProject(context.Context, projectcreate.CreateRequest) (projectcreate.Result, error) {
	return c.result, c.err
}

type retryableCreateError struct{}

func (retryableCreateError) Error() string            { return "upstream unavailable" }
func (retryableCreateError) Retryable() bool          { return true }
func (retryableCreateError) CorrectiveAction() string { return "Retry now." }

func TestCreatePreservesUnverifiedResultEvidence(t *testing.T) {
	for _, test := range []struct {
		name    string
		result  projectcreate.Result
		phase   errs.Phase
		outcome errs.Outcome
	}{
		{name: "observed identity", result: projectcreate.Result{Status: "unknown", Project: projectcreate.Project{LUID: "created-project", Name: "Unexpected"}, TableauRequestID: "accepted-request"}, phase: errs.PhaseVerification, outcome: errs.OutcomeUnknown},
		{name: "unreadable accepted response", result: projectcreate.Result{Status: "unknown", TableauRequestID: "accepted-request"}, phase: errs.PhaseSubmission, outcome: errs.OutcomeUnknown},
		{name: "pre-submission failure"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := projectcreate.New(&resolver{}, failedCreator{result: test.result, err: retryableCreateError{}}).Execute(t.Context(), projectcreate.Input{Environment: "dev", Site: "sandbox", Name: "Requested"}, false)
			if err == nil {
				t.Fatal("expected create failure")
			}
			payload := errs.Structure(err).Error
			if payload.Phase != test.phase || payload.Outcome != test.outcome || payload.Resource != test.result.Project.LUID || payload.TableauRequestID != test.result.TableauRequestID {
				t.Fatalf("payload = %#v", payload)
			}
			if test.outcome == errs.OutcomeUnknown {
				if payload.Retryable == nil || *payload.Retryable || payload.CorrectiveAction == "Retry now." || output.Result == nil || output.Result.Status != "unknown" {
					t.Fatalf("unsafe or lost outcome: payload=%#v output=%#v", payload, output)
				}
			} else if output.Result != nil {
				t.Fatalf("pre-submission output = %#v", output)
			}
			if test.result.Project.LUID != "" && !strings.Contains(payload.CorrectiveAction, "--project-id created-project") {
				t.Fatalf("missing exact inspect command: %#v", payload)
			}
		})
	}
}

func TestOutputGolden(t *testing.T) {
	output := projectcreate.Output{
		Plan: projectcreate.Plan{
			Mode: "execute", Operation: "project.create", Environment: "dev", Site: "sandbox",
			Project: projectcreate.ProjectSpec{Name: "Operations", Description: "Direct operations", ContentPermissions: "LockedToProject"},
			Parent:  &projectcreate.Project{LUID: "parent-1", Name: "Department", Path: "Department"},
		},
		Result: &projectcreate.Result{Status: "succeeded", Project: projectcreate.Project{LUID: "project-1", Name: "Operations", Path: "Department/Operations", ParentLUID: "parent-1", Description: "Direct operations", ContentPermissions: "LockedToProject"}, TableauRequestID: "request-1"},
		Help:   []string{"tadx content project inspect --project-id project-1"},
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

	preview, err := action.Execute(context.Background(), input, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Result != nil || c.calls != 0 || preview.Plan.Parent == nil || preview.Plan.Parent.LUID != "parent-1" {
		t.Fatalf("preview=%#v creator=%#v", preview, c)
	}

	result, err := action.Execute(context.Background(), input, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result == nil || c.calls != 1 || c.input.ParentLUID != "parent-1" || result.Result.Project.LUID != "project-1" {
		t.Fatalf("result=%#v creator=%#v", result, c)
	}
}

func TestCreateRootDoesNotResolveOrInferParent(t *testing.T) {
	r := &resolver{}
	c := &creator{}
	output, err := projectcreate.New(r, c).Execute(context.Background(), projectcreate.Input{Environment: "dev", Site: "sandbox", Name: "Root"}, false)
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
		_, err := projectcreate.New(r, c).Execute(context.Background(), projectcreate.Input{Environment: "dev", Site: "sandbox", Name: "operations"}, true)
		if err == nil || c.calls != 0 {
			t.Fatalf("error=%v calls=%d", err, c.calls)
		}
	})
	t.Run("parent identity changed", func(t *testing.T) {
		r := &resolver{parents: []projectcreate.Project{{LUID: "parent-1"}, {LUID: "parent-2"}}}
		c := &creator{}
		_, err := projectcreate.New(r, c).Execute(context.Background(), projectcreate.Input{Environment: "dev", Site: "sandbox", Name: "Operations", ParentSelector: identity.Selector{LUID: "parent-1"}}, false)
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
