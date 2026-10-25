package live

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func TestManifestIsNotReadyBeforeSetupFixtureObservationsComplete(t *testing.T) {
	state := t.TempDir()
	p := harness.Profile{ServerURL: "https://tableau.example.invalid", SiteContentURL: "test"}
	m := Manifest{Version: 1, FixtureSpec: fixtureSpecVersion, CatalogHash: CatalogHash(), Profile: p, SiteID: "site", RootProject: harness.Object{ID: "root", Name: "owned"}}
	m.TargetFingerprint = harness.Fingerprint(p.ServerURL, p.SiteContentURL, m.SiteID)
	path := filepath.Join(state, "manifest.json")
	if err := harness.WriteJSON(path, m); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(state, p); harness.ErrorStatus(err) != harness.BlockedStatus {
		t.Fatalf("unready setup permitted cases: %v", err)
	}
	m.ReadyAt = time.Now().UTC()
	if err := harness.WriteJSON(path, m); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(state, p); err != nil {
		t.Fatal(err)
	}
}

func TestFixtureCapabilityClosureContainsOnlyKnownRequiredHelpers(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "reference", "capabilities.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct{ ID string }
	if err = json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, row := range rows {
		known[row.ID] = true
	}
	catalog, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range catalog {
		for _, id := range fixtureCapabilities([]Spec{spec}) {
			if !known[id] {
				t.Errorf("%s fixture closure contains unknown capability %s", spec.ID, id)
			}
		}
	}
	plan, err := Select(Selectors{IDs: []string{"live.admin-label-category.list"}})
	if err != nil {
		t.Fatal(err)
	}
	specs, err := specsFor(plan)
	if err != nil {
		t.Fatal(err)
	}
	caps := fixtureCapabilities(specs)
	for _, unrelated := range []string{"content.label.update", "workbook.publish", "admin.group.create"} {
		if slices.Contains(caps, unrelated) {
			t.Errorf("category-only selection requires unrelated helper %s", unrelated)
		}
	}
}

func TestOutputResultSelectsRetainedWorkbookFixtureClosure(t *testing.T) {
	plan, err := Select(Selectors{IDs: []string{"live.contract.output-real-result"}})
	if err != nil {
		t.Fatal(err)
	}
	specs, err := specsFor(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !retainedWorkbookPackSelected(specs) {
		t.Fatal("output-result selection does not trigger retained workbook setup")
	}
	keys := selectedBindingKeys(specs)
	for _, key := range []string{"workbook-native", "workbook-inspect-primary", "workbook-inspect-decoy"} {
		if !keys[key] {
			t.Errorf("output-result setup omits retained fixture dependency %s", key)
		}
	}
	caps := fixtureCapabilities(specs)
	for _, capability := range []string{"workbook.publish", "workbook.delete", "job.inspect"} {
		if !slices.Contains(caps, capability) {
			t.Errorf("output-result setup omits required fixture capability %s", capability)
		}
	}

	unrelatedPlan, err := Select(Selectors{IDs: []string{"live.admin-label-category.list"}})
	if err != nil {
		t.Fatal(err)
	}
	unrelatedSpecs, err := specsFor(unrelatedPlan)
	if err != nil {
		t.Fatal(err)
	}
	if retainedWorkbookPackSelected(unrelatedSpecs) {
		t.Fatal("unrelated selection triggers retained workbook setup")
	}
	for key := range selectedBindingKeys(unrelatedSpecs) {
		if slices.Contains([]string{"workbook-native", "workbook-inspect-primary", "workbook-inspect-decoy"}, key) {
			t.Errorf("unrelated selection includes retained fixture dependency %s", key)
		}
	}
}

func TestOptionalFlowFixturesRequireTheirOwnAuthorizationAndHashes(t *testing.T) {
	plan, err := Select(Selectors{IDs: []string{"live.search.content-discovery"}})
	if err != nil {
		t.Fatal(err)
	}
	specs, err := specsFor(plan)
	if err != nil {
		t.Fatal(err)
	}
	p := harness.Profile{Features: map[string]string{"flows": "supported"}, Bindings: map[string]harness.Binding{"flow-native": {Path: filepath.Join(t.TempDir(), "missing.tflx")}}}
	caps := profileFixtureCapabilities(specs, p)
	for _, id := range []string{"flow.publish", "flow.delete"} {
		if !slices.Contains(caps, id) {
			t.Fatalf("flow fixture bypasses %s", id)
		}
	}
	if _, err := artifactHashesFor(p, specs); err == nil {
		t.Fatal("configured flow pack was omitted from selected input hashes")
	}
	p.Features = nil
	if slices.Contains(profileFixtureCapabilities(specs, p), "flow.publish") {
		t.Fatal("disabled optional flow still requires publication")
	}
	if _, err := artifactHashesFor(p, specs); err != nil {
		t.Fatalf("unselected optional flow blocks surgical setup: %v", err)
	}
}

func TestRecoveryManifestIgnoresMissingFixturesButPinsTarget(t *testing.T) {
	state := t.TempDir()
	p := harness.Profile{ServerURL: "https://tableau.example.invalid/", SiteContentURL: "test"}
	m := Manifest{Version: 1, CatalogHash: "old-catalog", FixtureSpec: "old-fixture-spec", SiteID: "site", Profile: p, ArtifactHashes: map[string]string{"binding:workbook-native": "missing"}}
	m.TargetFingerprint = harness.Fingerprint(p.ServerURL, p.SiteContentURL, m.SiteID)
	if err := harness.WriteJSON(filepath.Join(state, "manifest.json"), m); err != nil {
		t.Fatal(err)
	}
	if _, err := loadTargetManifest(state, p); err != nil {
		t.Fatalf("recovery must survive missing fixtures and catalog changes: %v", err)
	}
	if _, err := LoadManifest(state, p); err == nil {
		t.Fatal("ordinary runs accepted a stale setup")
	}
	p.SiteContentURL = "other"
	if _, err := loadTargetManifest(state, p); err == nil {
		t.Fatal("recovery accepted another exact site")
	}
}

func TestFixtureHashesFollowSelectedDependencyClosure(t *testing.T) {
	root := t.TempDir()
	native := filepath.Join(root, "fixture.twbx")
	if err := os.WriteFile(native, []byte("authored fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	p := harness.Profile{Bindings: map[string]harness.Binding{"workbook-native": {Path: native}, "flow-native": {Path: filepath.Join(root, "unavailable.tflx")}}}
	plan, err := Select(Selectors{IDs: []string{"live.workbook.pull"}})
	if err != nil {
		t.Fatal(err)
	}
	specs, err := specsFor(plan)
	if err != nil {
		t.Fatal(err)
	}
	hashes, err := artifactHashesFor(p, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 1 || hashes["binding:workbook-native"] == "" {
		t.Fatalf("unrelated optional inputs entered selection: %v", hashes)
	}
	m := Manifest{Profile: p, ArtifactHashes: hashes}
	if err := verifyArtifactHashes(m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(native, []byte("changed fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyArtifactHashes(m); err == nil {
		t.Fatal("changed native input was accepted")
	}
}

func TestSharedRootCleanupRequiresFinishedOwnedChildren(t *testing.T) {
	state := t.TempDir()
	run := "0123456789abcdef0123"
	j, err := harness.NewJournal(filepath.Join(state, "runs", run, "project.create", "journal.json"), run, "target", "tadx-live-owned")
	if err != nil {
		t.Fatal(err)
	}
	if err = j.AddIntent("project", "tadx-live-owned-child", "root"); err != nil {
		t.Fatal(err)
	}
	if err := guardRootCleanup(state, "target", "setup-run"); err == nil {
		t.Fatal("shared root could cascade an unresolved child")
	}
	j.Entries[0].Done = true
	if err := harness.WriteJSON(filepath.Join(state, "runs", run, "project.create", "journal.json"), j); err != nil {
		t.Fatal(err)
	}
	if err := guardRootCleanup(state, "target", "setup-run"); err != nil {
		t.Fatal(err)
	}
	j.TransferPending = true
	if err := harness.WriteJSON(filepath.Join(state, "runs", run, "project.create", "journal.json"), j); err != nil {
		t.Fatal(err)
	}
	if err := guardRootCleanup(state, "target", "setup-run"); err == nil {
		t.Fatal("unknown worker acceptance permitted shared root cleanup")
	}
}
