package capability

import "testing"

func TestBatchDeclarationsOnlySelectImplementedActions(t *testing.T) {
	for id := range BatchSelectors() {
		definition, ok := Lookup(id)
		if !ok || definition.Implementation != ImplementationImplemented || !definition.SupportsBatch {
			t.Errorf("invalid batch declaration %q", id)
		}
	}
	for _, id := range []string{"auth.login", "mutation.set", "agent.install", "env.add"} {
		if _, ok := BatchSelectors()[id]; ok {
			t.Errorf("global-state operation %q must not be batched", id)
		}
	}
}
