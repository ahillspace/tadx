package cli

import (
	"errors"
	"slices"
	"testing"

	"github.com/spf13/cobra"
)

func TestManagedPolicyBindingPreservesEnforcementOrder(t *testing.T) {
	denied := errors.New("denied")
	for _, tc := range []struct {
		name                                  string
		preview, denyCapability, denyMutation bool
		want                                  []string
	}{
		{name: "mutation", want: []string{"capability", "mutation", "run"}},
		{name: "preview", preview: true, want: []string{"capability", "run"}},
		{name: "capability denied", denyCapability: true, want: []string{"capability"}},
		{name: "preview denied", preview: true, denyCapability: true, want: []string{"capability"}},
		{name: "mutation denied", denyMutation: true, want: []string{"capability", "mutation"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			root := &cobra.Command{Use: "tadx"}
			child := &cobra.Command{Use: "delete", Annotations: map[string]string{CapabilityAnnotation: "workbook.delete"}, RunE: func(*cobra.Command, []string) error {
				events = append(events, "run")
				return nil
			}}
			child.Flags().Bool("preview", tc.preview, "")
			root.AddCommand(child)
			BindManagedPolicy(root, func(id string) error {
				if id != "workbook.delete" {
					t.Fatalf("capability=%q", id)
				}
				events = append(events, "capability")
				if tc.denyCapability {
					return denied
				}
				return nil
			}, func(id string) error {
				if id != "workbook.delete" {
					t.Fatalf("mutation=%q", id)
				}
				events = append(events, "mutation")
				if tc.denyMutation {
					return denied
				}
				return nil
			})
			err := child.RunE(child, nil)
			if !slices.Equal(events, tc.want) {
				t.Fatalf("events=%v want=%v", events, tc.want)
			}
			if (tc.denyCapability || tc.denyMutation) != errors.Is(err, denied) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestManagedPolicyBindingPreservesRootVersionAndRun(t *testing.T) {
	var events []string
	root := &cobra.Command{Use: "tadx", Run: func(*cobra.Command, []string) { events = append(events, "run") }}
	root.Flags().Bool("version", true, "")
	BindManagedPolicy(root, func(id string) error {
		events = append(events, id)
		return nil
	}, nil)
	if err := root.RunE(root, nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(events, []string{"version.get", "run"}) {
		t.Fatalf("events=%v", events)
	}
}
