package capability

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestCanonicalRegistryIsValidAndComplete(t *testing.T) {
	definitions := All()
	if got, want := len(definitions), 80; got != want {
		t.Fatalf("All() returned %d definitions, want %d", got, want)
	}
	if err := Validate(definitions); err != nil {
		t.Fatalf("Validate(All()) returned error: %v", err)
	}

	var cli, delegated, ship, blocked int
	for _, definition := range definitions {
		switch definition.Owner {
		case OwnerCLI:
			cli++
		default:
			delegated++
		}
		if definition.Disposition == DispositionShip {
			ship++
		}
		if definition.Verification == VerificationBlocked {
			blocked++
		}
	}
	if cli != 75 || delegated != 5 || ship != 75 || blocked != 8 {
		t.Fatalf("registry totals = cli:%d delegated:%d ship-disposition:%d blocked:%d, want 75/5/75/8", cli, delegated, ship, blocked)
	}
}

func TestCanonicalExecutableBindingsIncludeImplementedSlices(t *testing.T) {
	definitions := Executable()
	ids := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		ids = append(ids, definition.ID)
	}
	if want := []string{"admin.group.create", "admin.group.delete", "admin.group.get", "admin.group.list", "admin.group.update", "admin.permission.get", "admin.user.create", "admin.user.delete", "admin.user.get", "admin.user.list", "admin.user.update", "auth.check", "auth.status", "capability.get", "capability.list", "catalog.refresh", "catalog.search", "catalog.status", "datasource.delete", "datasource.get", "datasource.list", "datasource.publish", "datasource.pull", "datasource.schema", "doctor.run", "env.profile.add", "env.profile.get", "env.profile.list", "env.profile.remove", "env.profile.set-default", "env.profile.update", "flow.delete", "flow.get", "flow.list", "flow.move", "flow.publish", "flow.pull", "lineage.pull", "project.create", "project.get", "project.list", "project.update", "pulse.definition.create", "pulse.definition.get", "pulse.definition.list", "pulse.definition.pull", "pulse.metric.follow", "pulse.metric.followers", "pulse.metric.fork", "pulse.metric.get", "pulse.metric.list", "pulse.metric.unfollow", "workbook.delete", "workbook.get", "workbook.list", "workbook.publish", "workbook.pull", "workspace.artifact.delete", "workspace.clean", "workspace.clone", "workspace.create", "workspace.list", "workspace.move", "workspace.register", "workspace.status"}; !slices.Equal(ids, want) {
		t.Fatalf("Executable IDs = %v, want %v", ids, want)
	}
	for _, definition := range definitions {
		if definition.Implementation != ImplementationImplemented {
			t.Errorf("%s implementation = %q, want %q", definition.ID, definition.Implementation, ImplementationImplemented)
		}
		if len(definition.CommandPath) == 0 {
			t.Errorf("%s has no command path", definition.ID)
		}
	}
}

// TestExecutableCapabilitiesAreShipAndProven guards CLI availability against
// registry disposition rather than only against non-nil dependencies. Every
// capability wired to a runnable CLI command (Executable) must be dispositioned
// to ship and verification-ready, so a future misconfiguration that wires a
// blocked or non-ship capability into the manifest fails here instead of
// exposing an unproven command through the CLI.
func TestExecutableCapabilitiesAreShipAndProven(t *testing.T) {
	for _, definition := range Executable() {
		if definition.Disposition != DispositionShip {
			t.Errorf("%s is wired to the CLI but has disposition %q, want %q", definition.ID, definition.Disposition, DispositionShip)
		}
		if definition.Verification != VerificationReady {
			t.Errorf("%s is wired to the CLI but has verification %q, want %q", definition.ID, definition.Verification, VerificationReady)
		}
		if definition.Blocker != "" {
			t.Errorf("%s is wired to the CLI but references blocker %q", definition.ID, definition.Blocker)
		}
	}
}

func TestLookupIsExactAndReturnsCopy(t *testing.T) {
	definition, ok := Lookup("capability.get")
	if !ok {
		t.Fatal("Lookup(capability.get) did not find canonical definition")
	}
	if definition.ID != "capability.get" {
		t.Fatalf("Lookup returned %q", definition.ID)
	}
	if _, ok := Lookup("Capability.Get"); ok {
		t.Fatal("Lookup accepted a non-exact ID")
	}

	definition.CommandPath[0] = "changed"
	again, _ := Lookup("capability.get")
	if again.CommandPath[0] == "changed" {
		t.Fatal("Lookup exposed mutable canonical registry storage")
	}
}

func TestDefinitionJSONContainsIndependentStatusAndSafetyFields(t *testing.T) {
	definition, _ := Lookup("datasource.publish")
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	for _, field := range []string{
		`"disposition"`, `"evidence_level"`, `"verification"`, `"implementation"`,
		`"local_write"`, `"remote_mutation"`, `"requires_apply"`,
	} {
		if !strings.Contains(string(encoded), field) {
			t.Errorf("JSON does not contain %s: %s", field, encoded)
		}
	}
}

func TestValidateRejectsEveryRegistryInvariant(t *testing.T) {
	valid := testDefinition("sample.get")
	tests := []struct {
		name string
		defs []Definition
		want string
	}{
		{name: "duplicate IDs", defs: []Definition{valid, valid}, want: "duplicate capability ID"},
		{name: "duplicate implemented command paths", defs: []Definition{implemented(testDefinition("one.get"), "one", "get"), implemented(testDefinition("two.get"), "one", "get")}, want: "duplicate implemented command path"},
		{name: "missing required fields", defs: []Definition{{ID: "incomplete"}}, want: "missing required field"},
		{name: "remote mutation without apply", defs: []Definition{with(valid, func(d *Definition) { d.RemoteMutation = true })}, want: "remote mutation requires apply"},
		{name: "apply without consequential write", defs: []Definition{with(valid, func(d *Definition) { d.RequiresApply = true })}, want: "requires apply without consequential write"},
		{name: "delegated with binding", defs: []Definition{with(valid, func(d *Definition) {
			d.Disposition = DispositionDelegated
			d.Owner = OwnerMCP
			d.Implementation = ImplementationExternalDelegated
			d.CommandPath = []string{"sample", "get"}
		})}, want: "delegated capability has local command binding"},
		{name: "implemented CLI without binding", defs: []Definition{with(valid, func(d *Definition) { d.Implementation = ImplementationImplemented })}, want: "implemented CLI capability has no binding"},
		{name: "blocked executable", defs: []Definition{with(implemented(valid, "sample", "get"), func(d *Definition) { d.Verification = VerificationBlocked; d.Blocker = BlockerB1 })}, want: "blocked capability is executable"},
		{name: "invalid owner", defs: []Definition{with(valid, func(d *Definition) { d.Owner = Owner("unknown") })}, want: "invalid owner"},
		{name: "invalid blocker", defs: []Definition{with(valid, func(d *Definition) { d.Verification = VerificationBlocked; d.Blocker = BlockerID("B99") })}, want: "invalid blocker"},
		{name: "ready with blocker", defs: []Definition{with(valid, func(d *Definition) { d.Blocker = BlockerB1 })}, want: "ready capability references blocker"},
		{name: "blocked without blocker", defs: []Definition{with(valid, func(d *Definition) { d.Verification = VerificationBlocked })}, want: "blocked capability has no blocker"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.defs)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestValidateAllowsApplyForConsequentialLocalWrite(t *testing.T) {
	definition := with(testDefinition("workspace.artifact.delete"), func(d *Definition) {
		d.LocalWrite = true
		d.RequiresApply = true
	})
	if err := Validate([]Definition{definition}); err != nil {
		t.Fatalf("Validate() rejected consequential local write with apply: %v", err)
	}
}

func TestValidateBindingsRejectsBindingWithoutRegistryEntry(t *testing.T) {
	err := ValidateBindings([]Definition{testDefinition("sample.get")}, []Binding{{CapabilityID: "missing.get", CommandPath: []string{"missing", "get"}}})
	if err == nil || !strings.Contains(err.Error(), "binding has no registry entry") {
		t.Fatalf("ValidateBindings() error = %v, want orphan binding error", err)
	}
}

func TestValidateBindingsRejectsNonExecutableBindings(t *testing.T) {
	tests := []struct {
		name    string
		def     Definition
		binding Binding
		want    string
	}{
		{
			name:    "planned capability",
			def:     testDefinition("sample.get"),
			binding: Binding{CapabilityID: "sample.get", CommandPath: []string{"sample", "get"}},
			want:    "binding references non-implemented capability",
		},
		{
			name:    "empty binding path",
			def:     implemented(testDefinition("sample.get"), "sample", "get"),
			binding: Binding{CapabilityID: "sample.get"},
			want:    "binding has empty command path",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateBindings([]Definition{test.def}, []Binding{test.binding})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateBindings() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func testDefinition(id string) Definition {
	return Definition{
		ID:             id,
		Surface:        "tadx sample get",
		Outcome:        "Inspect one sample.",
		Type:           OperationInspect,
		Disposition:    DispositionShip,
		Owner:          OwnerCLI,
		Selectors:      "Sample ID",
		Availability:   "Local / all",
		SafetyGuard:    "Exact ID",
		ArtifactEffect: "None",
		Upstream:       "Local sample store",
		Evidence:       "Test evidence",
		EvidenceLevel:  EvidenceLocalContract,
		Verification:   VerificationReady,
		Implementation: ImplementationPlanned,
		Validation:     "Local contract",
	}
}

func implemented(definition Definition, path ...string) Definition {
	definition.Implementation = ImplementationImplemented
	definition.CommandPath = path
	return definition
}

func with(definition Definition, change func(*Definition)) Definition {
	change(&definition)
	return definition
}
