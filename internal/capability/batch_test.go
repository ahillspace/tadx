package capability

import (
	"reflect"
	"testing"
)

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

func TestBatchEnvironmentTargetsRemainExplicitAndNonRemote(t *testing.T) {
	want := map[string]bool{"auth.check": true, "auth.status": true, "auth.logout": true, "cache.refresh": true, "cache.status": true, "doctor.run": true}
	got := map[string]bool{}
	for id, options := range BatchOptions() {
		if options.AllowEnvironment {
			got[id] = true
			definition, ok := Lookup(id)
			if !ok || definition.RemoteMutation {
				t.Errorf("remote action %s cannot vary environment per row", id)
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment targets=%v", got)
	}
}

func TestBatchOptionsIncludeScopedCollectionsAndLocalTargets(t *testing.T) {
	options := BatchOptions()
	if len(options) != 95 {
		t.Fatalf("batch action count=%d, want 95", len(options))
	}
	for id, selector := range map[string]string{
		"project.list": "parent-id", "content.label.list": "target-id", "catalog.table.list": "database-id",
		"catalog.column.list": "table-id", "catalog.search": "table-id", "pulse.metric.list": "definition-id",
		"workspace.move": "artifact", "workspace.artifact.delete": "artifact", "workspace.status": "workspace",
	} {
		if len(options[id].Selectors) == 0 || options[id].Selectors[0] != selector {
			t.Errorf("%s selectors=%v", id, options[id].Selectors)
		}
	}
	for _, id := range []string{"workspace.create", "workspace.register", "workspace.clone", "workspace.delete", "workspace.unregister", "env.profile.add", "env.profile.get", "env.profile.update", "env.profile.remove", "capability.get", "search.run"} {
		if !options[id].Positional {
			t.Errorf("%s positional batch missing", id)
		}
	}
}
