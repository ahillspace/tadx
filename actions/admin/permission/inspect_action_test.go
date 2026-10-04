package permission_test

import (
	"context"
	"testing"

	permissionget "github.com/ahillspace/tadx/actions/admin/permission"
)

type reader struct {
	set permissionget.InspectPermissionSet
}

func (r reader) GetPermissions(context.Context, permissionget.InspectInput) (permissionget.InspectPermissionSet, error) {
	return r.set, nil
}

// TestExecuteDoesNotMutateReaderRules guards against in-place filtering that aliases
// and corrupts the reader-owned backing array (append-to-truncated-slice).
func TestExecuteDoesNotMutateReaderRules(t *testing.T) {
	original := []permissionget.InspectRule{
		{PrincipalType: "user", PrincipalLUID: "u-1", Capability: "Read", Mode: "Allow"},
		{PrincipalType: "group", PrincipalLUID: "g-1", Capability: "Write", Mode: "Deny"},
		{PrincipalType: "user", PrincipalLUID: "u-2", Capability: "Read", Mode: "Allow"},
	}
	backing := make([]permissionget.InspectRule, len(original))
	copy(backing, original)

	set := permissionget.InspectPermissionSet{ResourceKind: "workbook", ResourceLUID: "wb-1", Source: "explicit", Rules: backing}

	// Filter to only user principals so the filter drops at least one rule.
	in := permissionget.InspectInput{ResourceKind: "workbook", ResourceLUID: "wb-1", PrincipalType: "user"}
	out, err := permissionget.InspectPermissions(context.Background(), reader{set: set}, in)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(out.Permissions.Rules) != 2 {
		t.Fatalf("filtered rule count = %d, want 2", len(out.Permissions.Rules))
	}

	// The reader-owned backing array must be untouched by filtering.
	for i := range original {
		if backing[i] != original[i] {
			t.Fatalf("reader backing array mutated at %d: got %#v, want %#v", i, backing[i], original[i])
		}
	}
}

func TestFullOutputKeepsAllFetchedRules(t *testing.T) {
	rules := make([]permissionget.InspectRule, 201)
	for i := range rules {
		rules[i] = permissionget.InspectRule{PrincipalType: "user", PrincipalLUID: "u-" + string(rune(i+1)), Capability: "Read", Mode: "Allow"}
	}
	out, err := permissionget.InspectPermissions(context.Background(), reader{set: permissionget.InspectPermissionSet{ResourceKind: "workbook", ResourceLUID: "wb-1", Source: "explicit", Rules: rules}}, permissionget.InspectInput{ResourceKind: "workbook", ResourceLUID: "wb-1"})
	if err != nil {
		t.Fatal(err)
	}
	full := out.FullOutput().(permissionget.InspectFullResult)
	if len(full.Permissions.Rules) != len(rules) || full.Permissions.RulesOmitted != 0 {
		t.Fatalf("full rules = %d omitted=%d, want %d and 0", len(full.Permissions.Rules), full.Permissions.RulesOmitted, len(rules))
	}
}
