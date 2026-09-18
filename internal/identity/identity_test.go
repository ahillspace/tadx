package identity_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ahillspace/tadx/internal/identity"
)

func TestResolvePrefersAuthoritativeLUID(t *testing.T) {
	t.Parallel()

	candidates := []identity.Candidate{
		{LUID: "new-id", Name: "Renamed", ProjectPath: "Moved"},
		{LUID: "other-id", Name: "Old name", ProjectPath: "Old path"},
	}
	got, err := identity.Resolve(identity.Selector{LUID: "new-id", Name: "Old name", ProjectPath: "Old path"}, candidates)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.LUID != "new-id" {
		t.Fatalf("Resolve() LUID = %q", got.LUID)
	}
}

func TestResolveUsesLiteralExactNameAndProjectPath(t *testing.T) {
	t.Parallel()

	candidates := []identity.Candidate{
		{LUID: "sales-east", Name: "Sales", ProjectPath: "Regional/East"},
		{LUID: "sales-west", Name: "Sales", ProjectPath: "Regional/West"},
	}
	got, err := identity.Resolve(identity.Selector{Name: "Sales", ProjectPath: "Regional/East"}, candidates)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.LUID != "sales-east" {
		t.Fatalf("Resolve() LUID = %q", got.LUID)
	}

	_, err = identity.Resolve(identity.Selector{Name: "sales", ProjectPath: "Regional/East"}, candidates)
	assertResolutionKind(t, err, identity.ResolutionNotFound)
}

func TestResolveUsesLiteralExactNameAndProjectLUID(t *testing.T) {
	t.Parallel()

	candidates := []identity.Candidate{
		{LUID: "sales-east", Name: "Sales", ProjectLUID: "project-east"},
		{LUID: "sales-west", Name: "Sales", ProjectLUID: "project-west"},
	}
	got, err := identity.Resolve(identity.Selector{Name: "Sales", ProjectLUID: "project-east"}, candidates)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.LUID != "sales-east" {
		t.Fatalf("Resolve() LUID = %q", got.LUID)
	}
}

func TestResolveProjectByExactPath(t *testing.T) {
	t.Parallel()

	got, err := identity.Resolve(identity.Selector{ProjectPath: "Top/Child"}, []identity.Candidate{
		{LUID: "child", Name: "Child", ProjectPath: "Top/Child"},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.LUID != "child" {
		t.Fatalf("Resolve() LUID = %q", got.LUID)
	}
}

func TestResolveReturnsDeterministicZeroMatchError(t *testing.T) {
	t.Parallel()

	selector := identity.Selector{Name: "Missing", ProjectPath: "Top"}
	_, err := identity.Resolve(selector, nil)
	assertResolutionKind(t, err, identity.ResolutionNotFound)
	if got, want := err.Error(), `no resource matches name "Missing" in project "Top"`; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestResolveReturnsDeterministicAmbiguousError(t *testing.T) {
	t.Parallel()

	candidates := []identity.Candidate{
		{LUID: "z-id", Name: "Sales", ProjectPath: "Top"},
		{LUID: "a-id", Name: "Sales", ProjectPath: "Top"},
	}
	_, err := identity.Resolve(identity.Selector{Name: "Sales", ProjectPath: "Top"}, candidates)
	assertResolutionKind(t, err, identity.ResolutionAmbiguous)

	var resolutionErr *identity.ResolutionError
	if !errors.As(err, &resolutionErr) {
		t.Fatalf("error type = %T", err)
	}
	if want := []identity.LUID{"a-id", "z-id"}; !reflect.DeepEqual(resolutionErr.MatchLUIDs, want) {
		t.Fatalf("MatchLUIDs = %#v, want %#v", resolutionErr.MatchLUIDs, want)
	}
	if got, want := err.Error(), `resource selector name "Sales" in project "Top" is ambiguous; matches LUIDs [a-id, z-id]`; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestResolveCollapsesDuplicateRowsWithSameLUID(t *testing.T) {
	t.Parallel()

	candidate := identity.Candidate{LUID: "same-id", Name: "Sales", ProjectPath: "Top"}
	got, err := identity.Resolve(identity.Selector{Name: "Sales", ProjectPath: "Top"}, []identity.Candidate{candidate, candidate})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != candidate {
		t.Fatalf("Resolve() = %#v", got)
	}
}

func TestResolveRejectsEmptySelector(t *testing.T) {
	t.Parallel()

	_, err := identity.Resolve(identity.Selector{}, nil)
	assertResolutionKind(t, err, identity.ResolutionInvalidSelector)
}

func assertResolutionKind(t *testing.T, err error, want identity.ResolutionErrorKind) {
	t.Helper()
	var resolutionErr *identity.ResolutionError
	if !errors.As(err, &resolutionErr) {
		t.Fatalf("error = %v, want *identity.ResolutionError", err)
	}
	if resolutionErr.Kind != want {
		t.Fatalf("Kind = %q, want %q", resolutionErr.Kind, want)
	}
}
