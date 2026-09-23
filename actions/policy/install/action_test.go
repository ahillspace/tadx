package install_test

import (
	"context"
	"errors"
	"testing"

	policyinstall "github.com/ahillspace/tadx/actions/policy/install"
	"github.com/ahillspace/tadx/internal/errs"
)

type installer struct {
	input  policyinstall.Input
	output policyinstall.Output
	err    error
	calls  int
}

func (i *installer) InstallManagedPolicy(_ context.Context, input policyinstall.Input) (policyinstall.Output, error) {
	i.calls++
	i.input = input
	return i.output, i.err
}

func TestExecuteDefaultsToSuperuserAndPreservesReceipt(t *testing.T) {
	backend := &installer{output: policyinstall.Output{
		Path:              `C:/Program Files/TADX/managed-policy.json`,
		Template:          "superuser",
		ProtectionChanged: true,
		PolicyWritten:     true,
		LocatorPublished:  true,
		Active:            true,
		Phase:             "complete",
	}}
	action := policyinstall.New(backend)

	output, err := action.Execute(t.Context(), policyinstall.Input{OutputDirectory: `C:\Program Files\TADX`})
	if err != nil {
		t.Fatal(err)
	}
	if backend.calls != 1 || backend.input.Template != "superuser" || backend.input.OutputDirectory != `C:\Program Files\TADX` {
		t.Fatalf("installer calls=%d input=%+v", backend.calls, backend.input)
	}
	if output != backend.output {
		t.Fatalf("output=%+v want=%+v", output, backend.output)
	}
}

func TestExecuteCanonicalTemplatesAndLegacyAlias(t *testing.T) {
	for _, name := range []string{"read-only", "read-write-no-admin", "superuser", "admin", " admin "} {
		t.Run(name, func(t *testing.T) {
			backend := new(installer)
			_, err := policyinstall.New(backend).Execute(t.Context(), policyinstall.Input{Template: name})
			want := name
			if name == "admin" || name == " admin " {
				want = "superuser"
			}
			if err != nil || backend.calls != 1 || backend.input.Template != want {
				t.Fatalf("input=%+v calls=%d err=%v", backend.input, backend.calls, err)
			}
		})
	}
}

func TestExecuteRejectsUnknownTemplateBeforeInstall(t *testing.T) {
	backend := new(installer)
	_, err := policyinstall.New(backend).Execute(t.Context(), policyinstall.Input{Template: "owner"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "policy.install.usage" || structured.Kind != errs.KindUsage || backend.calls != 0 {
		t.Fatalf("error=%#v calls=%d", err, backend.calls)
	}
}

func TestExecutePreservesPartialReceiptAndError(t *testing.T) {
	wantErr := errors.New("locator publish failed")
	backend := &installer{
		output: policyinstall.Output{Path: `C:/Program Files/TADX/managed-policy.json`, Template: policyinstall.TemplateReadOnly, ProtectionChanged: true, PolicyWritten: true, Phase: "locator"},
		err:    wantErr,
	}

	output, err := policyinstall.New(backend).Execute(t.Context(), policyinstall.Input{Template: policyinstall.TemplateReadOnly})
	if !errors.Is(err, wantErr) || output != backend.output {
		t.Fatalf("output=%+v error=%v", output, err)
	}
}

func TestExecuteRequiresConfiguredInstaller(t *testing.T) {
	_, err := policyinstall.New(nil).Execute(t.Context(), policyinstall.Input{})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "policy.install.unconfigured" || structured.Outcome != errs.OutcomeNotAttempted {
		t.Fatalf("error=%#v", err)
	}
}
