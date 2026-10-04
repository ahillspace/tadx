package capability_test

import (
	"testing"

	"github.com/ahillspace/tadx/internal/capability"
)

func TestPolicyRecoveryOperationsAreExact(t *testing.T) {
	for _, id := range []string{"policy.install", "policy.samples", "policy.validate", "policy.status"} {
		if !capability.IsPolicyRecoveryOperation(id) {
			t.Fatalf("%q is not a policy recovery operation", id)
		}
	}
	for _, id := range []string{"policy.set", "policy.install.extra", "capability.list", ""} {
		if capability.IsPolicyRecoveryOperation(id) {
			t.Fatalf("%q is unexpectedly a policy recovery operation", id)
		}
	}
}
