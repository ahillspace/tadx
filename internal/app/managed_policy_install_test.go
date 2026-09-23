package app

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	policyinstall "github.com/ahillspace/tadx/actions/policy/install"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/managedpolicy"
)

func TestPolicyInstallIsAlwaysARecoveryOperation(t *testing.T) {
	if !policyRecoveryOperation("policy.install") {
		t.Fatal("policy.install is subject to the policy it must be able to repair")
	}
	runtime := &runtimeDependencies{managedPolicy: fixtureManagedPolicy{state: managedpolicy.StateError}}
	discovery, ok := (registrySource{runtime: runtime}).Get(t.Context(), "policy.install")
	if !ok || discovery.PolicyDenied {
		t.Fatalf("recovery discovery=%+v exists=%t", discovery, ok)
	}
	if err := runtime.checkManagedCapability("policy.install"); err != nil {
		t.Fatalf("recovery execution blocked: %v", err)
	}
}

func TestPolicyInstallErrorPreservesConfirmedPartialEffects(t *testing.T) {
	output := policyinstall.Output{
		Path:              "C:/Program Files/TADX/managed-policy.json",
		Template:          policyinstall.TemplateSuperuser,
		ProtectionChanged: true,
		PolicyWritten:     true,
		Active:            true,
		Phase:             "verify",
	}
	err := policyInstallError(output, errors.New("verification failed"))
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error=%T %v", err, err)
	}
	if structured.ID != "policy.install.failed" || structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeUnknown || structured.Retryable == nil || *structured.Retryable {
		t.Fatalf("error=%+v", structured)
	}
	if !slices.Equal(structured.Completed, []string{"directory_protection", "policy_write", "policy_active"}) {
		t.Fatalf("completed=%v", structured.Completed)
	}
}

func TestPolicyInstallErrorWithoutChangesIsNotAttempted(t *testing.T) {
	err := policyInstallError(policyinstall.Output{Phase: "validation"}, errors.New("unsupported platform"))
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Phase != errs.PhaseValidation || structured.Outcome != errs.OutcomeNotAttempted || len(structured.Completed) != 0 {
		t.Fatalf("error=%+v", structured)
	}
}

func TestPolicyInstallUnknownOrLocatorFailureDoesNotClaimNoMutation(t *testing.T) {
	for _, phase := range []string{"unknown", "locator"} {
		err := policyInstallError(policyinstall.Output{Phase: phase}, errors.New("helper stopped"))
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Outcome != errs.OutcomeUnknown {
			t.Fatalf("phase=%s error=%+v", phase, structured)
		}
	}
}

func TestPolicyInstallPartialReceiptRetainsDestinationWarningAndCause(t *testing.T) {
	cause := errors.New("locator publication failed")
	result := managedpolicy.InstallResult{Path: filepath.Join(t.TempDir(), "managed-policy.json"), Template: "read-only", PolicyWritten: true, Phase: "locator"}
	warning := "The selected policy path may be replaced and a different policy substituted."
	output, err := installedPolicyOutput(result, cause, []string{warning})
	if output.Path != filepath.ToSlash(result.Path) || !output.PolicyWritten || output.LocatorPublished || output.Active || output.Phase != "locator" || !slices.Equal(output.Warnings, []string{warning}) {
		t.Fatalf("partial receipt=%+v", output)
	}
	var structured *errs.Error
	if !errors.Is(err, cause) || !errors.As(err, &structured) || structured.Outcome != errs.OutcomeUnknown || structured.Phase != errs.PhasePersistence {
		t.Fatalf("original failure lost: %v", err)
	}
}
