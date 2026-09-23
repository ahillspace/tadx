package policy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	policyinstall "github.com/ahillspace/tadx/actions/policy/install"
	policycli "github.com/ahillspace/tadx/internal/cli/policy"
	"github.com/ahillspace/tadx/internal/output"
)

type installer struct {
	input  policyinstall.Input
	output policyinstall.Output
	err    error
	calls  int
}

func (i *installer) Execute(_ context.Context, input policyinstall.Input) (policyinstall.Output, error) {
	i.calls++
	i.input = input
	return i.output, i.err
}

type renderer struct {
	value any
	calls int
}

func (r *renderer) Render(value any) error {
	r.calls++
	r.value = value
	return nil
}

func TestInstallCommandDefaultsToSuperuserAndRendersReceipt(t *testing.T) {
	want := policyinstall.Output{Template: policyinstall.TemplateSuperuser, Phase: "complete", Active: true}
	install := &installer{output: want}
	render := new(renderer)
	command := policycli.New(policycli.Dependencies{Installer: install, Renderer: render})
	command.SetArgs([]string{"install"})

	if err := command.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if install.calls != 1 || install.input.Template != policyinstall.TemplateSuperuser || install.input.OutputDirectory != "" {
		t.Fatalf("installer calls=%d input=%+v", install.calls, install.input)
	}
	if render.calls != 1 || !reflect.DeepEqual(render.value, want) {
		t.Fatalf("render calls=%d value=%+v", render.calls, render.value)
	}
}

func TestInstallCommandPassesExplicitOutputAndTemplate(t *testing.T) {
	install := new(installer)
	render := new(renderer)
	command := policycli.New(policycli.Dependencies{Installer: install, Renderer: render})
	command.SetArgs([]string{"install", "--output", `D:\Managed TADX`, "--template", policyinstall.TemplateReadWriteNoAdmin})

	if err := command.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if install.calls != 1 || install.input.OutputDirectory != `D:\Managed TADX` || install.input.Template != policyinstall.TemplateReadWriteNoAdmin {
		t.Fatalf("installer calls=%d input=%+v", install.calls, install.input)
	}
}

func TestInstallCommandPreservesPartialOutputOnFailure(t *testing.T) {
	wantErr := errors.New("verification failed")
	want := policyinstall.Output{Template: policyinstall.TemplateReadOnly, PolicyWritten: true, Phase: "verify"}
	install := &installer{output: want, err: wantErr}
	command := policycli.New(policycli.Dependencies{Installer: install, Renderer: new(renderer)})
	command.SetArgs([]string{"install", "--template", policyinstall.TemplateReadOnly})

	err := command.ExecuteContext(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("error=%v", err)
	}
	retained, ok := err.(interface{ OperationOutput() any })
	if !ok || !reflect.DeepEqual(retained.OperationOutput(), want) {
		t.Fatalf("retained output=%#v error=%T %v", retained, err, err)
	}
}

func TestInstallPartialWarningRendersInCompactAndFullTOONAndJSON(t *testing.T) {
	wantErr := errors.New("locator publication failed")
	want := policyinstall.Output{Path: `C:/selected/managed-policy.json`, Template: policyinstall.TemplateReadOnly, PolicyWritten: true, Phase: "locator", Warnings: []string{"destination ancestor permits policy substitution"}}
	for _, full := range []bool{false, true} {
		for _, asJSON := range []bool{false, true} {
			command := policycli.New(policycli.Dependencies{Installer: &installer{output: want, err: wantErr}, Renderer: new(renderer)})
			command.SetArgs([]string{"install"})
			command.SilenceErrors = true
			command.SilenceUsage = true
			err := command.ExecuteContext(t.Context())
			if !errors.Is(err, wantErr) {
				t.Fatalf("original install failure lost: %v", err)
			}
			var rendered bytes.Buffer
			if err := output.RenderError(&rendered, err, output.Options{Full: full, JSON: asJSON}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(rendered.String(), "destination ancestor permits policy substitution") || !strings.Contains(rendered.String(), "locator publication failed") {
				t.Fatalf("full=%t json=%t partial output=%s", full, asJSON, &rendered)
			}
			if asJSON {
				var payload struct {
					Output policyinstall.Output `json:"output"`
				}
				if err := json.Unmarshal(rendered.Bytes(), &payload); err != nil || !reflect.DeepEqual(payload.Output, want) {
					t.Fatalf("full=%t JSON partial output=%s error=%v", full, &rendered, err)
				}
			}
		}
	}
}
