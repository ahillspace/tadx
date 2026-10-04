package policy

import (
	"errors"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/managedpolicy"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPolicyInstallErrorPreservesConfirmedPartialEffects(t *testing.T) {
	output := InstallOutput{
		Path:              "C:/Program Files/TADX/managed-policy.json",
		Template:          TemplateSuperuser,
		ProtectionChanged: true,
		PolicyWritten:     true,
		Active:            true,
		Phase:             "verify",
	}
	cause := errors.New("verification failed")
	err := policyInstallError(output, cause)
	structured, ok := errors.AsType[*errs.Error](err)
	if !ok {
		t.Fatalf("error=%T %v", err, err)
	}
	if structured.ID != "policy.install.failed" || structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeUnknown || structured.Retryable == nil || *structured.Retryable {
		t.Fatalf("error=%+v", structured)
	}
	if !slices.Equal(structured.Completed, []string{"directory_protection", "policy_write", "policy_active"}) {
		t.Fatalf("completed=%v", structured.Completed)
	}
	if !errors.Is(err, cause) || !strings.Contains(structured.CorrectiveAction, "tadx policy install --help") || !strings.Contains(structured.CorrectiveAction, "tadx policy status") {
		t.Fatalf("recovery lost original cause or status guidance: %+v", structured)
	}
}

func TestPolicyInstallErrorWithoutChangesIsNotAttempted(t *testing.T) {
	err := policyInstallError(InstallOutput{Phase: "validation"}, errors.New("unsupported platform"))
	structured, ok := errors.AsType[*errs.Error](err)
	if !ok || structured.Phase != errs.PhaseValidation || structured.Outcome != errs.OutcomeNotAttempted || len(structured.Completed) != 0 {
		t.Fatalf("error=%+v", structured)
	}
	if !strings.Contains(structured.CorrectiveAction, "tadx policy install --help") || strings.Contains(structured.CorrectiveAction, "rerun") {
		t.Fatalf("validation recovery=%q", structured.CorrectiveAction)
	}
}

func TestPolicyInstallUnknownOrLocatorFailureDoesNotClaimNoMutation(t *testing.T) {
	for _, phase := range []string{"unknown", "locator"} {
		err := policyInstallError(InstallOutput{Phase: phase}, errors.New("helper stopped"))
		structured, ok := errors.AsType[*errs.Error](err)
		if !ok || structured.Outcome != errs.OutcomeUnknown {
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
	structured, ok := errors.AsType[*errs.Error](err)
	if !errors.Is(err, cause) || !ok || structured.Outcome != errs.OutcomeUnknown || structured.Phase != errs.PhasePersistence {
		t.Fatalf("original failure lost: %v", err)
	}
}
