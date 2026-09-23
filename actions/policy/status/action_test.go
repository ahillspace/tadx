package status

import (
	"encoding/json"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

func TestStatusReportsActivePolicyWithUnsafeAncestor(t *testing.T) {
	out := Output{Policy: value.ManagedPolicyStatus{
		State:          "active",
		Protected:      true,
		PathProtected:  false,
		Warnings:       []string{"policy path may be replaced and a different policy substituted"},
		Checks:         []value.ManagedPolicyProtectionCheck{{Path: `C:\`, Kind: "ancestor-owner-acl-and-links", Reason: "cannot read owner and DACL"}},
		CandidateValid: true,
	}}
	for _, rendered := range []any{out.CompactOutput(), out.FullOutput()} {
		data, err := json.Marshal(rendered)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		if string(fields["protected"]) != "true" || string(fields["path_protected"]) != "false" || len(fields["warnings"]) == 0 {
			t.Fatalf("status hid the warning: %s", data)
		}
	}
}
