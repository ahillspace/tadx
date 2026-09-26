package workspace_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type registerRegistrar struct {
	input   workspaceaction.RegisterInput
	failure error
}

func (r *registerRegistrar) Register(_ context.Context, input workspaceaction.RegisterInput) (workspaceaction.Registration, error) {
	r.input = input
	if r.failure != nil {
		return workspaceaction.Registration{}, r.failure
	}
	name := input.Name
	if name == "" {
		name = "adopted"
	}
	return workspaceaction.Registration{Workspace: workspaceaction.Workspace{Name: name, ID: "ws_22222222222222222222222222222222", Root: "existing-root"}, ManifestVersion: 1, Registered: true}, nil
}

func TestRegisterExecuteAdoptsWorkspaceAndProjectsOutput(t *testing.T) {
	dependency := &registerRegistrar{}
	result, err := (&workspaceaction.Service{Registrar: dependency}).Register(t.Context(), workspaceaction.RegisterInput{Path: "existing-root", Name: "adopted"})
	if err != nil {
		t.Fatal(err)
	}
	if dependency.input.Path != "existing-root" || result.Status != "registered" || result.Workspace.Name != "adopted" {
		t.Fatalf("result = %#v, input = %#v", result, dependency.input)
	}
	var compact bytes.Buffer
	if err := output.Render(&compact, result); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, compact.Bytes(), "testdata/register/output.toon")
	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, result, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(full.Bytes(), []byte("existing-root")) {
		t.Fatalf("full output omits the resolved machine-local root:\n%s", full.String())
	}
	assertGolden(t, full.Bytes(), "testdata/register/output_full.toon")
}

func TestRegisterExecuteSurfacesRegistrationFailure(t *testing.T) {
	dependency := &registerRegistrar{failure: errors.New("workspace manifest is incomplete")}
	_, err := (&workspaceaction.Service{Registrar: dependency}).Register(t.Context(), workspaceaction.RegisterInput{Path: "not-a-workspace"})
	structured, ok := errors.AsType[*errs.Error](err)
	if !ok || structured.ID != "workspace.register.failed" || structured.Kind != errs.KindOperation {
		t.Fatalf("error = %#v", err)
	}
}
