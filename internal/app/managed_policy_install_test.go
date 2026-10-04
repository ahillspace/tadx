package app

import (
	capabilityops "github.com/ahillspace/tadx/actions/capability"
	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/managedpolicy"
	"testing"
)

func TestPolicyInstallIsAlwaysARecoveryOperation(t *testing.T) {
	if !capability.IsPolicyRecoveryOperation("policy.install") {
		t.Fatal("policy.install is subject to the policy it must be able to repair")
	}
	runtime := &runtimeDependencies{managedPolicy: fixtureManagedPolicy{state: managedpolicy.StateError}}
	output, err := capabilityops.New(capabilityops.Ports{ManagedPolicy: runtime.managedPolicy}).GetCapability(t.Context(), capabilityops.GetInput{ID: "policy.install"})
	if err != nil || output.Capability.PolicyDenied {
		t.Fatalf("recovery discovery=%+v err=%v", output.Capability, err)
	}
	if err := runtime.checkManagedCapability("policy.install"); err != nil {
		t.Fatalf("recovery execution blocked: %v", err)
	}
}
