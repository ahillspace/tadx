package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// SupportsPublicationExecution identifies commands with detached process support.
func SupportsPublicationExecution(operation string) bool {
	switch operation {
	case "workbook.publish", "datasource.publish", "flow.publish", "workbook.pull", "datasource.pull", "flow.pull":
		return true
	default:
		return false
	}
}

// PublicationExecution binds process coordination to long-running commands.
type PublicationExecution struct {
	Supports func(string) bool
	Worker   bool
	Enabled  bool
	Inline   func(noWait bool) error
	Begin    func(context.Context, string, string, bool, io.Writer) (ExecutionOutcome, error)
	Capture  func(any)
}

// ExecutionOutcome distinguishes captured results from error-attached output.
type ExecutionOutcome struct {
	Value    any
	Present  bool
	Rendered bool
}

// BindPublicationExecution decorates only supported publishing and pull actions.
// Cobra argument guards precede handoff; workers retain command policy checks.
func BindPublicationExecution(root *cobra.Command, execution PublicationExecution) {
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		if execution.Supports(command.Annotations[CapabilityAnnotation]) && command.RunE != nil {
			original := command.RunE
			command.RunE = func(cmd *cobra.Command, args []string) error {
				preview, _ := cmd.Flags().GetBool("preview")
				if preview || execution.Worker {
					return original(cmd, args)
				}
				noWait, _ := cmd.Flags().GetBool("no-wait")
				if !execution.Enabled {
					if err := execution.Inline(noWait); err != nil {
						return err
					}
					return original(cmd, args)
				}
				batchPath, _ := cmd.Flags().GetString("batch-file")
				result, err := execution.Begin(cmd.Context(), cmd.Annotations[CapabilityAnnotation], batchPath, noWait, cmd.ErrOrStderr())
				if result.Present {
					if err != nil && !result.Rendered {
						return clierr.WithOutput(result.Value, err)
					}
					execution.Capture(result.Value)
				}
				if err != nil && result.Rendered {
					return clierr.Rendered(err)
				}
				return err
			}
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
}

// OperationActivity formats the resource-specific foreground activity.
func OperationActivity(operation string) string {
	kind, verb, _ := strings.Cut(operation, ".")
	if verb == "pull" {
		return "Downloading " + kind
	}
	return "Publishing " + kind
}

// OperationBatchActivity formats a persisted batch progress observation.
func OperationBatchActivity(operation string, succeeded, pending, failed int) string {
	return fmt.Sprintf("%s: %d completed, %d pending, %d failed", OperationActivity(operation), succeeded, pending, failed)
}
