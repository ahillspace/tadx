package content

import (
	"context"
	"errors"
	"path"

	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/spf13/cobra"
)

// runContentSelection preserves the existing single-item result contract.
func runContentSelection[T any](ctx context.Context, operation string, selectors []string, renderer Renderer, execute func(context.Context, string) (T, error)) error {
	if len(selectors) <= 1 {
		selector := ""
		if len(selectors) == 1 {
			selector = selectors[0]
		}
		result, err := execute(ctx, selector)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return renderer.Render(result)
	}
	result, err := contentbatch.Run(ctx, operation, selectors, execute)
	if renderErr := renderer.Render(result); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return clierr.Rendered(err)
	}
	return nil
}

// runPublishSelection reports the active artifact while preserving the single
// and batch result contracts implemented by runContentSelection.
func runPublishSelection[T any](ctx context.Context, operation, kind string, artifacts []string, preview bool, renderer Renderer, reporter *progress.Reporter, publish func(context.Context, string) (T, error)) error {
	active := progress.OperationFromContext(ctx)
	if active == nil {
		active = reporter.Start(ctx, publishPreparationLabel(kind, preview))
		ctx = progress.WithOperation(ctx, active)
		defer active.Stop()
	}
	progress.SetLabel(ctx, publishSelectionLabel(kind, preview))
	return runContentSelection(ctx, operation, artifacts, renderer, func(ctx context.Context, artifact string) (T, error) {
		progress.SetLabel(ctx, publishActivityLabel(kind, artifact, preview))
		return publish(ctx, artifact)
	})
}

func publishPreparationLabel(kind string, preview bool) string {
	if preview {
		return "Preparing " + kind + " publication preview"
	}
	return "Preparing " + kind + " publication"
}

func publishSelectionLabel(kind string, preview bool) string {
	if preview {
		return "Selecting " + kind + " publication source"
	}
	return "Selecting " + kind + " source"
}

func publishActivityLabel(kind, artifact string, preview bool) string {
	name := path.Base(artifact)
	if artifact == "" || name == "." || name == "/" {
		if preview {
			return "Previewing " + kind + " publication"
		}
		return "Publishing " + kind
	}
	if preview {
		return "Previewing " + kind + " publication " + name
	}
	return "Publishing " + kind + " " + name
}

func batchPullArgs(operation string, ids *[]string, name, projectPath *string, set func(string, string, string)) cobra.PositionalArgs {
	return func(command *cobra.Command, args []string) error {
		if err := noContentArgs(operation)(command, args); err != nil {
			return err
		}
		if len(*ids) > 0 {
			if err := contentbatch.Validate(*ids); err != nil {
				return clierr.Usage(operation, err)
			}
			if *name != "" || *projectPath != "" {
				return clierr.Usage(operation, errors.New("use either --id or exact --name and --project"))
			}
			set((*ids)[0], "", "")
			return nil
		}
		luid := ""
		return selectorArgs(operation, &luid, name, projectPath, set)(command, args)
	}
}

func validateArtifactSelection(paths []string, kind, name string) error {
	if err := contentbatch.Validate(paths); err != nil {
		return err
	}
	if len(paths) > 1 && name != "" {
		return errors.New("--name requires one --artifact; batch publication preserves each artifact name")
	}
	for _, path := range paths {
		if err := validateManagedArtifactPath(path, kind); err != nil {
			return err
		}
	}
	return nil
}
