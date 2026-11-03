package batchinput

import (
	"reflect"
	"testing"
)

func TestPublicationExpectationsSpanRowsInSelectionOrder(t *testing.T) {
	saved := []PublishDestination{
		{Environment: "test", Site: "site", ProjectLUID: "p1", Name: "First"},
		{Environment: "test", Site: "site", ProjectLUID: "p2", Name: "Second", ExistingLUID: "w2"},
	}
	parent := EnsurePublicationPreparation(WithPublicationExpectations(t.Context(), saved))
	for i := range saved {
		row, _ := Collect(parent, "workbook.publish")
		expected, err := NextPublicationExpectation(row)
		if err != nil || expected == nil || *expected != saved[i] {
			t.Fatalf("row=%d expected=%v err=%v", i, expected, err)
		}
		if err := RecordPublication(row, *expected); err != nil {
			t.Fatal(err)
		}
		if i == 0 && CheckPublicationPreparation(parent) == nil {
			t.Fatal("unconsumed saved destination was accepted")
		}
	}
	if err := CheckPublicationPreparation(parent); err != nil {
		t.Fatal(err)
	}
	prepared := PreparedPublications(parent)
	if !reflect.DeepEqual(prepared, saved) {
		t.Fatalf("prepared=%v", prepared)
	}
	prepared[0].Name = "unrelated caller change"
	if PreparedPublications(parent)[0] != saved[0] {
		t.Fatal("caller mutated retained destinations")
	}
	if _, err := NextPublicationExpectation(parent); err == nil {
		t.Fatal("extra destination exceeded saved selection")
	}
}

func TestPublicationExpectationCompatibilityDistinguishesMissingAndEmpty(t *testing.T) {
	legacy := WithPublicationExpectations(t.Context(), nil)
	if expected, err := NextPublicationExpectation(legacy); expected != nil || err != nil {
		t.Fatalf("expected=%v err=%v", expected, err)
	}
	if HasPublicationExpectations(legacy) || CheckPublicationPreparation(legacy) != nil {
		t.Fatal("legacy record acquired a new destination requirement")
	}
	empty := WithPublicationExpectations(t.Context(), []PublishDestination{})
	if !HasPublicationExpectations(empty) {
		t.Fatal("explicit empty saved selection was discarded")
	}
	if _, err := NextPublicationExpectation(empty); err == nil {
		t.Fatal("explicitly empty saved selection allowed a destination")
	}
	if err := RecordPublication(t.Context(), PublishDestination{}); err == nil {
		t.Fatal("missing preparation context silently discarded a destination")
	}
}
