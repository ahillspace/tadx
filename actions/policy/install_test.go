package policy_test

import (
	"errors"
	"testing"

	"github.com/ahillspace/tadx/actions/policy"
	"github.com/ahillspace/tadx/internal/errs"
)

func TestNormalizeInstallDefaultsAndPreservesDirectory(t *testing.T) {
	input, err := policy.NormalizeInstall(policy.InstallInput{OutputDirectory: `C:\Program Files\TADX`})
	if err != nil || input.Template != "superuser" || input.OutputDirectory != `C:\Program Files\TADX` {
		t.Fatalf("input=%+v error=%v", input, err)
	}
}

func TestNormalizeInstallCanonicalTemplatesAndLegacyAlias(t *testing.T) {
	for _, name := range []string{"read-only", "read-write-no-admin", "superuser", "admin", " admin "} {
		t.Run(name, func(t *testing.T) {
			input, err := policy.NormalizeInstall(policy.InstallInput{Template: name})
			want := name
			if name == "admin" || name == " admin " {
				want = "superuser"
			}
			if err != nil || input.Template != want {
				t.Fatalf("input=%+v error=%v", input, err)
			}
		})
	}
}

func TestNormalizeInstallRejectsUnknownTemplate(t *testing.T) {
	_, err := policy.NormalizeInstall(policy.InstallInput{Template: "owner"})
	structured, ok := errors.AsType[*errs.Error](err)
	if !ok || structured.ID != "policy.install.usage" || structured.Kind != errs.KindUsage {
		t.Fatalf("error=%v", err)
	}
}
