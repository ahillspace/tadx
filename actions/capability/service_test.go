package capability

import (
	"errors"
	"testing"
)

type testManagedPolicy struct {
	deniedID string
	err      error
	checks   []string
}

func (p *testManagedPolicy) CheckCapability(id string) error {
	p.checks = append(p.checks, id)
	if id == p.deniedID {
		return p.err
	}
	return nil
}

func (p *testManagedPolicy) CheckRemoteMutation() error { return nil }

func TestServiceAppliesManagedAndSiteReadinessToExactDiscovery(t *testing.T) {
	denied := errors.New("managed denial")
	policy := &testManagedPolicy{deniedID: "admin.user.list", err: denied}
	service := New(Ports{ManagedPolicy: policy, ResolveMutationPolicy: func(environment string) (bool, string, error) {
		if environment != "qa" {
			t.Fatalf("environment = %q", environment)
		}
		return true, "enabled", nil
	}})
	output, err := service.GetCapability(t.Context(), GetInput{ID: "admin.user.list", Environment: "qa"})
	if err != nil || !output.Capability.PolicyDenied || output.Capability.PolicyReason != denied.Error() || output.Capability.ExecutionEnabled {
		t.Fatalf("output=%#v err=%v", output, err)
	}
	if len(policy.checks) != 1 || policy.checks[0] != "admin.user.list" {
		t.Fatalf("checks=%v", policy.checks)
	}
}

func TestServiceReturnsStaticListWithUnavailablePolicy(t *testing.T) {
	policyErr := errors.New("site policy unavailable")
	service := New(Ports{ResolveMutationPolicy: func(string) (bool, string, error) { return true, "", policyErr }})
	output, err := service.ListCapabilities(t.Context(), ListInput{Limit: 1, Mutation: new(true)})
	if !errors.Is(err, policyErr) || output.MutationPolicy != "unavailable" || len(output.Capabilities) != 1 || output.Capabilities[0].ExecutionEnabled {
		t.Fatalf("output=%#v err=%v", output, err)
	}
	invalid, err := service.ListCapabilities(t.Context(), ListInput{Cursor: "invalid"})
	if !errors.Is(err, policyErr) || invalid.MutationPolicy != "" {
		t.Fatalf("invalid output=%#v err=%v", invalid, err)
	}
}

func TestServiceGetPolicyFailurePrecedesIDValidation(t *testing.T) {
	policyErr := errors.New("site policy unavailable")
	service := New(Ports{ResolveMutationPolicy: func(string) (bool, string, error) { return false, "", policyErr }})
	_, err := service.GetCapability(t.Context(), GetInput{})
	if !errors.Is(err, policyErr) {
		t.Fatalf("error=%v", err)
	}
}

func TestServiceSkipsManagedPolicyForRecoveryDiscovery(t *testing.T) {
	policy := &testManagedPolicy{deniedID: "policy.install", err: errors.New("denied")}
	output, err := New(Ports{ManagedPolicy: policy}).GetCapability(t.Context(), GetInput{ID: "policy.install"})
	if err != nil || output.Capability.PolicyDenied || len(policy.checks) != 0 {
		t.Fatalf("output=%#v err=%v checks=%v", output, err, policy.checks)
	}
}
