package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/spf13/cobra"
)

type mutationPolicyFunc func(string) bool

func (f mutationPolicyFunc) IsRemoteMutation(id string) bool { return f(id) }

type nilMutationPolicy struct{}

func (*nilMutationPolicy) IsRemoteMutation(string) bool { return true }

func TestApplyMutationExecutionPolicyFailsClosedWithoutPolicy(t *testing.T) {
	var typedNil *nilMutationPolicy
	for name, policy := range map[string]MutationPolicy{"nil": nil, "typed nil": typedNil} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			root := &cobra.Command{Use: "tadx", SilenceErrors: true, SilenceUsage: true}
			command := &cobra.Command{
				Use:         "registered",
				Annotations: map[string]string{CapabilityAnnotation: "future.mutation"},
				RunE: func(*cobra.Command, []string) error {
					calls++
					return nil
				},
			}
			root.AddCommand(command)
			applyMutationExecutionPolicy(root, policy, true)
			root.SetArgs([]string{"registered"})
			err := root.Execute()

			var structured *errs.Error
			if !errors.As(err, &structured) || structured.ID != "mutation.policy.unconfigured" || structured.Kind != errs.KindRuntime || calls != 0 {
				t.Fatalf("error = %#v, calls = %d", err, calls)
			}
		})
	}
}

func TestRootFailsConfigurationWhenMutationPolicyIsMissing(t *testing.T) {
	root := NewRoot(Dependencies{ListUse: "list", ListShort: "List capabilities.", GetUse: "get <id>", GetShort: "Get a capability."})
	root.SetArgs([]string{"capability", "list"})
	err := root.Execute()
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "mutation.policy.unconfigured" {
		t.Fatalf("error = %#v", err)
	}
}

func TestConfiguredPolicyPreservesNonmutationExecution(t *testing.T) {
	calls := 0
	root := &cobra.Command{Use: "tadx", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(&cobra.Command{Use: "read", Annotations: map[string]string{CapabilityAnnotation: "content.read"}, Run: func(*cobra.Command, []string) { calls++ }})
	applyMutationExecutionPolicy(root, mutationPolicyFunc(func(string) bool { return false }), false)
	root.SetArgs([]string{"read"})
	if err := root.Execute(); err != nil || calls != 1 {
		t.Fatalf("error = %v, calls = %d", err, calls)
	}
}

func TestApplyMutationExecutionPolicyGatesFutureRegistryCommand(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			calls := 0
			root := &cobra.Command{Use: "tadx", SilenceErrors: true, SilenceUsage: true}
			future := &cobra.Command{
				Use:         "future",
				Hidden:      true,
				Annotations: map[string]string{CapabilityAnnotation: "future.mutation"},
				RunE: func(*cobra.Command, []string) error {
					calls++
					return nil
				},
			}
			root.AddCommand(future)
			applyMutationExecutionPolicy(root, mutationPolicyFunc(func(id string) bool { return id == "future.mutation" }), enabled)

			if future.Hidden {
				t.Fatal("registry-defined mutation command is hidden")
			}
			root.SetArgs([]string{"future"})
			err := root.Execute()
			if enabled {
				if err != nil || calls != 1 {
					t.Fatalf("enabled error = %v, calls = %d", err, calls)
				}
				return
			}
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.ID != "mutation.disabled" || structured.Operation != "future.mutation" || calls != 0 {
				t.Fatalf("disabled error = %#v, calls = %d", err, calls)
			}
		})
	}
}

func TestRootHelpExplainsDiscoveryOutputAndMutationSafety(t *testing.T) {
	root := NewRoot(Dependencies{})
	for _, value := range []string{"tadx capability list", "compact TOON", "--full", "TADX_ENABLE_MUTATIONS=1", "preview", "--apply"} {
		if !strings.Contains(root.Long, value) {
			t.Errorf("root help missing %q:\n%s", value, root.Long)
		}
	}
}
