package capability

import "testing"

func TestBatchDeclarationsOnlySelectImplementedActions(t *testing.T) {
	for id := range BatchSelectors() {
		definition, ok := Lookup(id)
		if !ok || definition.Implementation != ImplementationImplemented || !definition.SupportsBatch {
			t.Errorf("invalid batch declaration %q", id)
		}
	}
	for _, id := range []string{"auth.login", "mutation.set", "mutation.status", "env.profile.set-default", "workspace.set-default", "update"} {
		if _, ok := BatchSelectors()[id]; ok {
			t.Errorf("global-state operation %q must not be batched", id)
		}
	}
}

func TestUtilityAndSetupActionsRemainSingleOperations(t *testing.T) {
	options := BatchOptions()
	for _, id := range []string{
		"agent.install", "agent.uninstall", "auth.check", "auth.status", "auth.logout",
		"env.profile.add", "env.profile.get", "env.profile.update", "env.profile.remove",
		"cache.refresh", "cache.status", "capability.get", "doctor.run", "search.run", "catalog.search",
		"workspace.create", "workspace.register", "workspace.clone", "workspace.status",
	} {
		if _, ok := options[id]; ok {
			t.Errorf("single operation %q must not be batched", id)
		}
		definition, ok := Lookup(id)
		if !ok || definition.SupportsBatch {
			t.Errorf("single operation %q must not advertise batching", id)
		}
	}
}

func TestBatchOptionsIncludeScopedCollectionsAndLocalTargets(t *testing.T) {
	options := BatchOptions()
	for id, selector := range map[string]string{
		"project.list": "parent-id", "content.label.list": "target-id", "catalog.table.list": "database-id",
		"catalog.column.list": "table-id", "pulse.metric.list": "definition-id",
		"workspace.move": "artifact", "workspace.artifact.delete": "artifact", "workspace.clean": "workspace",
	} {
		if len(options[id].Selectors) == 0 || options[id].Selectors[0] != selector {
			t.Errorf("%s selectors=%v", id, options[id].Selectors)
		}
	}
	for _, id := range []string{"workspace.delete", "workspace.unregister"} {
		if !options[id].Positional {
			t.Errorf("%s positional batch missing", id)
		}
	}
}
