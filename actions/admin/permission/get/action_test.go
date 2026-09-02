package get_test

import (
	"context"
	"testing"

	permissionget "github.com/ahillspace/tadx/actions/admin/permission/get"
)

type reader struct{ set permissionget.PermissionSet }

func (r reader) GetPermissions(context.Context, permissionget.Input) (permissionget.PermissionSet, error) {
	return r.set, nil
}

// TestExecuteDoesNotMutateReaderRules guards against in-place filtering that aliases
// and corrupts the reader-owned backing array (append-to-truncated-slice).
func TestExecuteDoesNotMutateReaderRules(t *testing.T) {
	original := []permissionget.Rule{
		{PrincipalType: "user", PrincipalLUID: "u-1", Capability: "Read", Mode: "Allow"},
		{PrincipalType: "group", PrincipalLUID: "g-1", Capability: "Write", Mode: "Deny"},
		{PrincipalType: "user", PrincipalLUID: "u-2", Capability: "Read", Mode: "Allow"},
	}
	backing := make([]permissionget.Rule, len(original))
	copy(backing, original)

	set := permissionget.PermissionSet{ResourceKind: "workbook", ResourceLUID: "wb-1", Source: "explicit", Rules: backing}

	// Filter to only user principals so the filter drops at least one rule.
	in := permissionget.Input{ResourceKind: "workbook", ResourceLUID: "wb-1", PrincipalType: "user"}
	out, err := permissionget.New(reader{set: set}).Execute(context.Background(), in)
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
