package pulse

import (
	"errors"

	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func newDefinitionDelete(deps Dependencies) *cobra.Command {
	var input pulsedefinition.DeleteInput
	command := exactIDCommand("delete", "Delete one exact Pulse definition.", "pulse.definition.delete", &input.LUID, func(command *cobra.Command) error {
		if input.Environment == "" {
			return usage("pulse.definition.delete", "--environment is required")
		}
		result, err := deps.DefinitionDeleter.DeletePulseDefinition(command.Context(), input)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().BoolVar(&input.Preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func newDefinitionList(deps Dependencies) *cobra.Command {
	var input pulsedefinition.ListInput
	command := actionCommand("list", "List Pulse definitions.", "pulse.definition.list", func(command *cobra.Command) error {
		result, err := deps.DefinitionLister.ListPulseDefinitions(command.Context(), input)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	readFlags(command, &input.Environment, &input.Cache)
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum definitions to return from 1 through 10000; defaults to 25")
	command.Flags().StringVar(&input.Name, "name", "", "find exact definition names across provider pages")
	command.Flags().StringVar(&input.DatasourceLUID, "datasource-id", "", "filter exact datasource LUID before the returned limit; scans up to 100 pages")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	_ = command.Flags().MarkHidden("cursor")
	command.Flags().BoolVar(&input.All, "all", false, "return all matching definitions within 100 pages and 10,000 records; cannot combine with --limit")
	command.MarkFlagsMutuallyExclusive("all", "limit")
	command.MarkFlagsMutuallyExclusive("all", "cursor")
	return command
}

func newDefinitionInspect(deps Dependencies) *cobra.Command {
	var input pulsedefinition.InspectInput
	command := exactIDCommand("inspect", "Inspect one exact Pulse definition.", "pulse.definition.inspect", &input.LUID, func(command *cobra.Command) error {
		result, err := deps.DefinitionInspector.InspectPulseDefinition(command.Context(), input)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	readFlags(command, &input.Environment, &input.Cache)
	return command
}

func newDefinitionPull(deps Dependencies) *cobra.Command {
	var input pulsedefinition.PullInput
	command := exactIDCommand("pull", "Pull a portable definition bundle with every saved metric variant.", "pulse.definition.pull", &input.LUID, func(command *cobra.Command) error {
		result, err := deps.DefinitionPuller.PullPulseDefinition(command.Context(), input)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	command.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace a dirty local artifact")
	command.Flags().BoolVar(&input.Preview, "preview", false, "resolve acquisition scope and local conflicts without writing artifacts")
	return command
}

func newDefinitionPublish(deps Dependencies) *cobra.Command {
	var input pulsedefinition.PublishInput
	command := actionCommand("publish", "Recreate a portable Pulse bundle as new definitions and metrics.", "pulse.definition.publish", func(command *cobra.Command) error {
		if deps.DefinitionPublisher == nil {
			return errors.New("Pulse bundle publisher is not configured")
		}
		result, err := deps.DefinitionPublisher.PublishPulseDefinition(command.Context(), input)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	command.Flags().StringVar(&input.Environment, "environment", "", "destination environment alias")
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace containing the bundle")
	command.Flags().StringVar(&input.Artifact, "artifact", "", "workspace-relative managed Pulse bundle directory")
	command.Flags().StringVar(&input.ArtifactID, "id", "", "exact source definition LUID of the managed bundle; exclusive with --artifact-name and --artifact")
	command.Flags().StringVar(&input.ArtifactName, "artifact-name", "", "exact managed Pulse bundle name; ambiguity fails; exclusive with --id and --artifact")
	command.Flags().StringArrayVar(&input.DatasourceMap, "datasource-map", nil, "explicit source=destination datasource LUID mapping; repeat for each source, including same-site publishing")
	command.Flags().BoolVar(&input.Preview, "preview", false, "validate and preview every recreated object without remote writes")
	return command
}

func newDefinitionCreate(deps Dependencies) *cobra.Command {
	var input pulsedefinition.CreateInput
	var preview bool
	command := &cobra.Command{
		Use:         "create",
		Short:       "Create one Pulse definition.",
		Annotations: capability("pulse.definition.create"),
		Args: func(command *cobra.Command, args []string) error {
			if err := noArgs("pulse.definition.create", command, args); err != nil {
				return err
			}
			if input.Environment == "" || input.Intent.Name == "" || input.Intent.DatasourceLUID == "" || input.Intent.MeasureField == "" || input.Intent.TimeDimension == "" {
				return usage("pulse.definition.create", "--environment, --name, --datasource-id, --measure-field, and --date-field are required")
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.DefinitionCreator.CreatePulseDefinition(command.Context(), input, preview)
			if err != nil {
				return clierr.WithOutput(result, err)
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&input.Intent.Name, "name", "", "definition name")
	command.Flags().StringVar(&input.Intent.Description, "description", "", "definition description")
	command.Flags().StringVar(&input.Intent.DatasourceLUID, "datasource-id", "", "authoritative published datasource LUID")
	command.Flags().StringVar(&input.Intent.MeasureField, "measure-field", "", "exact Tableau measure field ID or unique display name; resolved to the raw ID")
	command.Flags().StringVar(&input.Intent.Aggregation, "aggregation", "", "aggregation: SUM, AVERAGE, MIN, MAX, COUNT, COUNT_DISTINCT, or USER; defaults to SUM")
	command.Flags().StringVar(&input.Intent.TimeDimension, "date-field", "", "exact Tableau date field ID or unique display name; resolved to the raw ID")
	command.Flags().StringArrayVar(&input.Intent.AllowedDimensions, "dimension", nil, "exact Tableau dimension field ID or unique display name; repeat for each allowed dimension")
	command.Flags().StringVar(&input.Intent.MinimumGranularity, "minimum-granularity", "", "minimum date granularity: DAY, WEEK, MONTH, QUARTER, or YEAR; defaults to DAY")
	command.Flags().StringVar(&input.Intent.NumberFormat, "number-format", "", "number format: NUMBER, CURRENCY, or PERCENT; defaults to NUMBER")
	command.Flags().StringVar(&input.Intent.CurrencyCode, "currency", "", "three-letter currency code when --number-format is CURRENCY; defaults to USD")
	command.Flags().StringVar(&input.Intent.Sentiment, "sentiment", "", "sentiment: UP, DOWN, or NONE; defaults to NONE")
	command.Flags().StringVar(&input.Intent.Temporality, "temporality", "", "temporality: OVER_TIME or LATEST; defaults to OVER_TIME")
	command.Flags().BoolVar(&input.Intent.RunningTotal, "running-total", false, "create a running total; requires SUM and OVER_TIME")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}
