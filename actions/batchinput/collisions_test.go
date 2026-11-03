package batchinput

import "testing"

func TestPublicationTargetsRetainResourceAndSlotOwnership(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		t.Run(kind, func(t *testing.T) {
			base := PublishDestination{Environment: "test", Site: "site-1", ProjectLUID: "project-1", Name: "\u03a3", ExistingLUID: "resource-1"}
			for _, tc := range []struct {
				name     string
				change   func(*PublishDestination)
				conflict bool
			}{
				{"same slot Unicode alias", func(d *PublishDestination) { d.Name = "\u03c2" }, true},
				{"same slot changed observed resource", func(d *PublishDestination) { d.ExistingLUID = "resource-2" }, true},
				{"same resource changed observed slot", func(d *PublishDestination) { d.ProjectLUID = "project-2"; d.Name = "Moved" }, true},
				{"independent destination", func(d *PublishDestination) { d.Name = "Other"; d.ExistingLUID = "resource-2" }, false},
				{"independent project", func(d *PublishDestination) { d.ProjectLUID = "project-2"; d.ExistingLUID = "resource-2" }, false},
				{"independent site", func(d *PublishDestination) { d.Site = "site-2" }, false},
				{"independent environment", func(d *PublishDestination) { d.Environment = "other" }, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					other := base
					tc.change(&other)
					conflict := false
					for _, first := range PublicationTargets(kind, base) {
						for _, second := range PublicationTargets(kind, other) {
							conflict = conflict || first == second
						}
					}
					if conflict != tc.conflict {
						t.Fatalf("conflict=%t want=%t", conflict, tc.conflict)
					}
				})
			}
			base.ExistingLUID = ""
			other := base
			other.Name = "\u03c2"
			if PublicationTargets(kind, base)[0] != PublicationTargets(kind, other)[0] {
				t.Fatal("absent destinations lost Unicode equivalence")
			}
			if base.Name != "\u03a3" || other.Name != "\u03c2" {
				t.Fatal("execution names changed")
			}
		})
	}
}

func TestNamedTargetsRespectSiblingScope(t *testing.T) {
	base := NamedTarget("test", "site", "project", "parent-1", "\u03a3")
	if base != NamedTarget("test", "site", "project", "parent-1", "\u03c2") {
		t.Fatal("sibling aliases are independent")
	}
	for _, other := range []Target{
		NamedTarget("test", "site", "project", "parent-2", "\u03c2"),
		NamedTarget("test", "other", "project", "parent-1", "\u03c2"),
		NamedTarget("other", "site", "project", "parent-1", "\u03c2"),
		NamedTarget("test", "site", "project", "parent-1", "Other"),
	} {
		if base == other {
			t.Fatalf("independent scope conflicts: %+v", other)
		}
	}
}
