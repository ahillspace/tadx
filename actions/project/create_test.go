package project_test

import (
	"bytes"
	"context"
	"errors"
	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type createResolver struct {
	parents        []projectops.CreateProject
	collisions     []projectops.CreateProject
	parentCall     int
	lateCollisions []projectops.CreateProject
	collisionCalls int
}

func (r *createResolver) BeginProjectResolution(ctx context.Context) context.Context { return ctx }

func (r *createResolver) ResolveProject(context.Context, identity.Selector) (projectops.CreateProject, error) {
	if len(r.parents) == 0 {
		return projectops.CreateProject{}, errors.New("missing parent")
	}
	index := r.parentCall
	if index >= len(r.parents) {
		index = len(r.parents) - 1
	}
	r.parentCall++
	return r.parents[index], nil
}

func (r *createResolver) FindProjectCollisions(context.Context, string, string) ([]projectops.CreateProject, error) {
	r.collisionCalls++
	if r.collisionCalls > 1 && r.lateCollisions != nil {
		return r.lateCollisions, nil
	}
	return append([]projectops.CreateProject(nil), r.collisions...), nil
}

func TestCreateRejectsLateSiblingCollisionBeforeWriting(t *testing.T) {
	r := &createResolver{lateCollisions: []projectops.CreateProject{{LUID: "other", Name: "Operations"}}}
	c := &createCreator{}
	_, err := newTestService(projectops.Ports{CreateResolver: r, Creator: c}).CreateProject(t.Context(), projectops.CreateInput{Environment: "dev", Site: "sandbox", Name: "Operations"}, false)
	if err == nil || !strings.Contains(err.Error(), "already") || r.collisionCalls != 2 || c.calls != 0 {
		t.Fatalf("error=%v collision_reads=%d writes=%d", err, r.collisionCalls, c.calls)
	}
}

type createCreator struct {
	calls int
	input projectops.CreateRequest
}

type createFailedCreator struct {
	result projectops.CreateResult
	err    error
}

func (c createFailedCreator) CreateProject(context.Context, projectops.CreateRequest) (projectops.CreateResult, error) {
	return c.result, c.err
}

type createRetryableCreateError struct{}

func (createRetryableCreateError) Error() string            { return "upstream unavailable" }
func (createRetryableCreateError) Retryable() bool          { return true }
func (createRetryableCreateError) CorrectiveAction() string { return "Retry now." }

func TestCreatePreservesUnverifiedResultEvidence(t *testing.T) {
	for _, test := range []struct {
		name    string
		result  projectops.CreateResult
		phase   errs.Phase
		outcome errs.Outcome
	}{
		{name: "observed identity", result: projectops.CreateResult{Status: "unknown", Project: projectops.CreateProject{LUID: "created-project", Name: "Unexpected"}, TableauRequestID: "accepted-request"}, phase: errs.PhaseVerification, outcome: errs.OutcomeUnknown},
		{name: "unreadable accepted response", result: projectops.CreateResult{Status: "unknown", TableauRequestID: "accepted-request"}, phase: errs.PhaseSubmission, outcome: errs.OutcomeUnknown},
		{name: "pre-submission failure"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := newTestService(projectops.Ports{CreateResolver: &createResolver{}, Creator: createFailedCreator{result: test.result, err: createRetryableCreateError{}}}).CreateProject(t.Context(), projectops.CreateInput{Environment: "dev", Site: "sandbox", Name: "Requested"}, false)
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

func TestCreateOutputGolden(t *testing.T) {
	output := projectops.CreateOutput{
		Plan: projectops.CreatePlan{
			Mode: "execute", Operation: "project.create", Environment: "dev", Site: "sandbox",
			Project: projectops.CreateProjectSpec{Name: "Operations", Description: "Direct operations", ContentPermissions: "LockedToProject"},
			Parent:  &projectops.CreateProject{LUID: "parent-1", Name: "Department", Path: "Department"},
		},
		Result: &projectops.CreateResult{Status: "succeeded", Project: projectops.CreateProject{LUID: "project-1", Name: "Operations", Path: "Department/Operations", ParentLUID: "parent-1", Description: "Direct operations", ContentPermissions: "LockedToProject"}, TableauRequestID: "request-1"},
		Help:   []string{"tadx content project inspect --project-id project-1"},
	}
	createAssertGolden(t, "compact.toon", output, false)
	createAssertGolden(t, "full.toon", output, true)
}

func createAssertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata/create", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

func (c *createCreator) CreateProject(_ context.Context, input projectops.CreateRequest) (projectops.CreateResult, error) {
	c.calls++
	c.input = input
	return projectops.CreateResult{Status: "succeeded", Project: projectops.CreateProject{LUID: "project-1", Name: input.Name, ParentLUID: input.ParentLUID, Path: "Department/" + input.Name}, TableauRequestID: "request-1"}, nil
}

func TestCreatePreviewsThenRevalidatesParentAndCollisionOnApply(t *testing.T) {
	r := &createResolver{parents: []projectops.CreateProject{{LUID: "parent-1", Name: "Department", Path: "Department"}, {LUID: "parent-1", Name: "Renamed", Path: "Renamed"}}}
	c := &createCreator{}
	action := newTestService(projectops.Ports{CreateResolver: r, Creator: c})
	input := projectops.CreateInput{Environment: "dev", Site: "sandbox", Name: "Operations", Description: "Direct operations", ContentPermissions: "LockedToProject", ParentSelector: identity.Selector{LUID: "parent-1"}}

	preview, err := action.CreateProject(context.Background(), input, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Result != nil || c.calls != 0 || preview.Plan.Parent == nil || preview.Plan.Parent.LUID != "parent-1" {
		t.Fatalf("preview=%#v creator=%#v", preview, c)
	}

	result, err := action.CreateProject(context.Background(), input, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result == nil || c.calls != 1 || c.input.ParentLUID != "parent-1" || result.Result.Project.LUID != "project-1" {
		t.Fatalf("result=%#v creator=%#v", result, c)
	}
}

func TestCreateRootDoesNotResolveOrInferParent(t *testing.T) {
	r := &createResolver{}
	c := &createCreator{}
	output, err := newTestService(projectops.Ports{CreateResolver: r, Creator: c}).CreateProject(context.Background(), projectops.CreateInput{Environment: "dev", Site: "sandbox", Name: "Root"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if r.parentCall != 0 || c.input.ParentLUID != "" || output.Plan.Parent != nil {
		t.Fatalf("output=%#v resolver=%#v creator=%#v", output, r, c)
	}
}

func TestCreateRejectsCollisionAndChangedParentIdentity(t *testing.T) {
	t.Run("collision", func(t *testing.T) {
		r := &createResolver{collisions: []projectops.CreateProject{{LUID: "existing", Name: "Operations"}}}
		c := &createCreator{}
		_, err := newTestService(projectops.Ports{CreateResolver: r, Creator: c}).CreateProject(context.Background(), projectops.CreateInput{Environment: "dev", Site: "sandbox", Name: "operations"}, true)
		if err == nil || c.calls != 0 {
			t.Fatalf("error=%v calls=%d", err, c.calls)
		}
	})
	t.Run("parent identity changed", func(t *testing.T) {
		r := &createResolver{parents: []projectops.CreateProject{{LUID: "parent-1"}, {LUID: "parent-2"}}}
		c := &createCreator{}
		_, err := newTestService(projectops.Ports{CreateResolver: r, Creator: c}).CreateProject(context.Background(), projectops.CreateInput{Environment: "dev", Site: "sandbox", Name: "Operations", ParentSelector: identity.Selector{LUID: "parent-1"}}, false)
		if err == nil || c.calls != 0 {
			t.Fatalf("error=%v calls=%d", err, c.calls)
		}
	})
}

func TestCreateRequiresExplicitTargetAndValidFields(t *testing.T) {
	tests := []projectops.CreateInput{
		{Name: "Operations"},
		{Environment: "dev", Site: "sandbox"},
		{Environment: "dev", Site: "sandbox", Name: "Operations", ContentPermissions: "invalid"},
		{Environment: "dev", Site: "sandbox", Name: "Operations/Reports"},
		{Environment: "dev", Site: "sandbox", Name: "Operations", ParentSelector: identity.Selector{LUID: "parent-1", ProjectPath: "Department"}},
	}
	for _, input := range tests {
		if _, err := newTestService(projectops.Ports{CreateResolver: &createResolver{}, Creator: &createCreator{}}).CreateProject(context.Background(), input, false); err == nil {
			t.Fatalf("input accepted: %#v", input)
		}
	}
}

func TestCreateCompactOutputOmitsSuccessfulRequestID(t *testing.T) {
	output := projectops.CreateOutput{Result: &projectops.CreateResult{Status: "succeeded", Project: projectops.CreateProject{LUID: "project-1", Name: "Operations"}, TableauRequestID: "request-secret"}}
	compact := output.CompactOutput().(projectops.CreateCompactResult)
	if compact.Result == nil || compact.Result.Project.LUID != "project-1" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
}
