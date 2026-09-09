package content

import (
	"context"
	"errors"

	lineagepull "github.com/ahillspace/tadx/actions/lineage/pull"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type LineagePuller interface {
	PullLineage(context.Context, lineagepull.Input) (lineagepull.Output, error)
}

func newLineage(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "lineage", Short: "Capture bounded Tableau lineage"}
	var input lineagepull.Input
	var luid, name, projectPath string
	pull := &cobra.Command{Use: "pull", Short: "Pull bounded lineage without a native artifact.", Annotations: map[string]string{"tadx.capability": "lineage.pull"}, Args: func(command *cobra.Command, args []string) error {
		if err := noContentArgs("lineage.pull")(command, args); err != nil {
			return err
		}
		if input.Kind != "workbook" && input.Kind != "datasource" && input.Kind != "published_datasource" && input.Kind != "flow" {
			return clierr.Usage("lineage.pull", errors.New("--kind must be workbook, datasource, or flow"))
		}
		if luid != "" {
			if name != "" || projectPath != "" {
				return clierr.Usage("lineage.pull", errors.New("use either --id or an exact name selector"))
			}
			input.SetSelector(luid, "", "")
		} else {
			if name == "" || projectPath == "" {
				return clierr.Usage("lineage.pull", errors.New("name selection requires both --name and --project"))
			}
			input.SetSelector("", name, projectPath)
		}
		return nil
	}, RunE: func(command *cobra.Command, _ []string) error {
		result, err := deps.LineagePuller.PullLineage(command.Context(), input)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	}}
	pull.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	pull.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	pull.Flags().StringVar(&input.Kind, "kind", "", "lineage root kind: workbook, datasource, or flow")
	pull.Flags().StringVar(&luid, "id", "", "authoritative REST LUID")
	pull.Flags().StringVar(&name, "name", "", "exact resource name")
	pull.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	pull.Flags().StringVar(&input.Direction, "direction", "both", "lineage direction: upstream, downstream, or both")
	pull.Flags().IntVar(&input.Depth, "depth", 1, "bounded lineage depth from 1 to 3")
	pull.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace a dirty metadata-only lineage artifact")
	command.AddCommand(pull)
	return command
}
