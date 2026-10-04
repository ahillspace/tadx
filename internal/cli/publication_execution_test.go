package cli

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/spf13/cobra"
)

func TestPublicationExecutionSupportsOnlyNativeContentTransfers(t *testing.T) {
	for _, operation := range []string{"workbook.publish", "datasource.publish", "flow.publish", "workbook.pull", "datasource.pull", "flow.pull"} {
		if !SupportsPublicationExecution(operation) {
			t.Fatalf("supported transfer rejected: %s", operation)
		}
	}
	for _, operation := range []string{"", "workbook.delete", "pulse.definition.publish", "catalog.lineage.pull", "WORKBOOK.PUBLISH", "workbook.publish.extra"} {
		if SupportsPublicationExecution(operation) {
			t.Fatalf("unsupported command admitted: %s", operation)
		}
	}
}

func TestPublicationExecutionPreservesPreviewAndWorkerBypass(t *testing.T) {
	for _, test := range []struct {
		name            string
		preview, worker bool
	}{
		{"preview", true, false}, {"worker", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			command := &cobra.Command{Use: "publish", Annotations: map[string]string{CapabilityAnnotation: "workbook.publish"}, RunE: func(*cobra.Command, []string) error { calls++; return nil }}
			command.Flags().Bool("preview", test.preview, "")
			BindPublicationExecution(command, PublicationExecution{
				Supports: func(string) bool { return true }, Worker: test.worker, Enabled: true,
				Begin: func(context.Context, string, string, bool, io.Writer) (ExecutionOutcome, error) {
					t.Fatal("unexpected handoff")
					return ExecutionOutcome{}, nil
				},
			})
			if err := command.RunE(command, nil); err != nil || calls != 1 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
}

func TestPublicationExecutionPreservesArgumentGuardBeforeHandoff(t *testing.T) {
	command := &cobra.Command{Use: "publish", Annotations: map[string]string{CapabilityAnnotation: "workbook.publish"}, Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error { t.Fatal("unexpected original action"); return nil }}
	BindPublicationExecution(command, PublicationExecution{
		Supports: func(string) bool { return true }, Enabled: true,
		Begin: func(context.Context, string, string, bool, io.Writer) (ExecutionOutcome, error) {
			t.Fatal("unexpected handoff")
			return ExecutionOutcome{}, nil
		},
	})
	command.SetArgs([]string{"unexpected"})
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	if err := command.ExecuteContext(t.Context()); err == nil {
		t.Fatal("invalid args accepted")
	}
}

func TestPublicationExecutionCapturesFailedCompletedOutput(t *testing.T) {
	expected := errors.New("completed with failure")
	value := struct{ ID string }{ID: "saved-operation"}
	var captured any
	command := &cobra.Command{Use: "publish", Annotations: map[string]string{CapabilityAnnotation: "workbook.publish"}, RunE: func(*cobra.Command, []string) error { t.Fatal("unexpected original action"); return nil }}
	BindPublicationExecution(command, PublicationExecution{
		Supports: func(string) bool { return true }, Enabled: true,
		Begin: func(context.Context, string, string, bool, io.Writer) (ExecutionOutcome, error) {
			return ExecutionOutcome{Value: value, Present: true, Rendered: true}, expected
		},
		Capture: func(v any) { captured = v },
	})
	if err := command.RunE(command, nil); !errors.Is(err, expected) || captured != value {
		t.Fatalf("err=%v captured=%v", err, captured)
	}
}
