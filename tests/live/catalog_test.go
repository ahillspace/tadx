package live

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCatalogAccountsForEveryCapability(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "reference", "capabilities.json"))
	if err != nil {
		t.Fatal(err)
	}
	var caps []struct {
		ID, Implementation string
		RemoteMutation     bool `json:"remote_mutation"`
	}
	if err = json.Unmarshal(data, &caps); err != nil {
		t.Fatal(err)
	}
	catalog, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	exclusions, err := Exclusions()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, test := range catalog {
		for _, cap := range test.Capabilities {
			seen[cap] = true
		}
	}
	for _, cap := range caps {
		if !seen[cap.ID] && exclusions[cap.ID] == "" {
			t.Errorf("capability %s has neither live coverage nor an explicit exclusion", cap.ID)
		}
	}
	known := map[string]bool{}
	for _, cap := range caps {
		known[cap.ID] = true
	}
	for id := range seen {
		if !known[id] {
			t.Errorf("unknown capability %s", id)
		}
	}
}
func TestEveryApprovedCaseHasExactlyOneExecutableScenario(t *testing.T) {
	all, err := Scenarios()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 130 {
		t.Fatalf("approved case count changed without catalog review: %d", len(all))
	}
}

func TestSelectionRequiresExplicitKnownSelector(t *testing.T) {
	for _, s := range []Selectors{{}, {IDs: []string{"live.no-such.case"}}, {Capabilities: []string{"no-such-capability"}}, {Groups: []string{"no-such-group"}}} {
		if _, err := Select(s); err == nil {
			t.Errorf("selector %#v unexpectedly succeeds", s)
		}
	}
	p, err := Select(Selectors{IDs: []string{"live.datasource.pull"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tests) != 1 || p.Tests[0].ID != "live.datasource.pull" {
		t.Fatalf("single-case selection broadened: %#v", p.Tests)
	}
}

func TestChangedPathSelectionTracksSharedConsumers(t *testing.T) {
	p, err := Select(Selectors{ChangedPaths: []string{"actions/datasource/pull.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tests) == 0 {
		t.Fatal("datasource owner selected no tests")
	}
	catalog, _ := Catalog()
	for _, test := range p.Tests {
		idx := slices.IndexFunc(catalog, func(s Spec) bool { return s.ID == test.ID })
		if idx < 0 || !strings.HasPrefix(catalog[idx].Group, "datasource") && catalog[idx].Group != "job" {
			t.Errorf("unrelated test selected: %s", test.ID)
		}
	}
	if _, err = Select(Selectors{ChangedPaths: []string{"internal/new-production-boundary/new.go"}}); err == nil {
		t.Fatal("unknown production owner silently selected zero tests")
	}
	docs, err := Select(Selectors{ChangedPaths: []string{"docs/new-guide.md"}})
	if err != nil || len(docs.Tests) != 0 {
		t.Fatalf("docs-only selection: %v %v", docs, err)
	}
	shared, err := Select(Selectors{ChangedPaths: []string{"internal/tableau/transport.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(shared.Tests) <= len(p.Tests) {
		t.Fatal("shared transport did not select its broader consumer set")
	}
}

func TestPlanRejectsChangedCatalogAndUnknownTests(t *testing.T) {
	p, err := Select(Selectors{IDs: []string{"live.workbook.pull"}})
	if err != nil {
		t.Fatal(err)
	}
	p.CatalogHash = "old"
	if err = p.Validate(); err == nil {
		t.Fatal("stale plan accepted")
	}
	p, err = Select(Selectors{IDs: []string{"live.workbook.pull"}})
	if err != nil {
		t.Fatal(err)
	}
	p.Tests[0].ID = "live.unknown.case"
	if err = p.Validate(); err == nil {
		t.Fatal("unknown test accepted from plan file")
	}
}
