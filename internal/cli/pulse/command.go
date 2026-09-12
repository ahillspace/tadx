// Package pulse contains thin Cobra plumbing for Tableau Pulse lifecycle commands.
package pulse

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	definitioncreate "github.com/ahillspace/tadx/actions/pulse/definition/create"
	definitiondelete "github.com/ahillspace/tadx/actions/pulse/definition/delete"
	definitioninspect "github.com/ahillspace/tadx/actions/pulse/definition/inspect"
	definitionlist "github.com/ahillspace/tadx/actions/pulse/definition/list"
	definitionpublish "github.com/ahillspace/tadx/actions/pulse/definition/publish"
	definitionpull "github.com/ahillspace/tadx/actions/pulse/definition/pull"
	metricdelete "github.com/ahillspace/tadx/actions/pulse/metric/delete"
	metricfollow "github.com/ahillspace/tadx/actions/pulse/metric/follow"
	metricfollowers "github.com/ahillspace/tadx/actions/pulse/metric/followers"
	metricfork "github.com/ahillspace/tadx/actions/pulse/metric/fork"
	metricinspect "github.com/ahillspace/tadx/actions/pulse/metric/inspect"
	metriclist "github.com/ahillspace/tadx/actions/pulse/metric/list"
	metricunfollow "github.com/ahillspace/tadx/actions/pulse/metric/unfollow"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// Renderer writes one structured result.
type Renderer interface{ Render(any) error }

// DefinitionLister lists Pulse definitions.
type DefinitionLister interface {
	ListPulseDefinitions(context.Context, definitionlist.Input) (definitionlist.Output, error)
}

// DefinitionInspector inspects one Pulse definition.
type DefinitionInspector interface {
	InspectPulseDefinition(context.Context, definitioninspect.Input) (definitioninspect.Output, error)
}

// DefinitionPuller pulls one Pulse definition artifact.
type DefinitionPuller interface {
	PullPulseDefinition(context.Context, definitionpull.Input) (definitionpull.Output, error)
}

type DefinitionPublisher interface {
	PublishPulseDefinition(context.Context, definitionpublish.Input) (definitionpublish.Output, error)
}

// DefinitionCreator creates one Pulse definition or returns a preview.
type DefinitionCreator interface {
	CreatePulseDefinition(context.Context, definitioncreate.Input, bool) (definitioncreate.Output, error)
}

// DefinitionDeleter deletes one Pulse definition or returns a preview.
type DefinitionDeleter interface {
	DeletePulseDefinition(context.Context, definitiondelete.Input) (definitiondelete.Output, error)
}

// MetricLister lists the metrics for one definition.
type MetricLister interface {
	ListPulseMetrics(context.Context, metriclist.Input) (metriclist.Output, error)
}

// MetricInspector inspects one Pulse metric.
type MetricInspector interface {
	InspectPulseMetric(context.Context, metricinspect.Input) (metricinspect.Output, error)
}

// MetricForker creates one Pulse metric variant or returns a preview.
type MetricForker interface {
	ForkPulseMetric(context.Context, metricfork.Input, bool) (metricfork.Output, error)
}

// MetricDeleter deletes one Pulse metric or returns a preview.
type MetricDeleter interface {
	DeletePulseMetric(context.Context, metricdelete.Input) (metricdelete.Output, error)
}

// MetricFollowers lists one metric's subscriptions.
type MetricFollowers interface {
	ListPulseMetricFollowers(context.Context, metricfollowers.Input) (metricfollowers.Output, error)
}

// MetricFollower creates one Pulse subscription or returns a preview.
type MetricFollower interface {
	FollowPulseMetric(context.Context, metricfollow.Input, bool) (metricfollow.Output, error)
}

// MetricUnfollower removes one Pulse subscription or returns a preview.
type MetricUnfollower interface {
	UnfollowPulseMetric(context.Context, metricunfollow.Input, bool) (metricunfollow.Output, error)
}

// Dependencies contains Pulse command wiring.
type Dependencies struct {
	DefinitionLister    DefinitionLister
	DefinitionInspector DefinitionInspector
	DefinitionPuller    DefinitionPuller
	DefinitionPublisher DefinitionPublisher
	DefinitionCreator   DefinitionCreator
	DefinitionDeleter   DefinitionDeleter
	MetricLister        MetricLister
	MetricInspector     MetricInspector
	MetricForker        MetricForker
	MetricDeleter       MetricDeleter
	MetricFollowers     MetricFollowers
	MetricFollower      MetricFollower
	MetricUnfollower    MetricUnfollower
	Renderer            Renderer
}

// New creates the Pulse command tree.
func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "pulse",
		Short: "Manage Tableau Pulse definitions and metrics",
		Long: "Manage Tableau Pulse definitions, metric variants, and followers.\n\n" +
			"Discover source fields with tadx content datasource schema --id <datasource-luid> --query <term> before creating a definition.\n\n" +
			"TADX reads and changes saved Pulse configuration; it does not retrieve current metric values or generated insights.",
	}
	definition := &cobra.Command{Use: "definition", Short: "Manage Pulse metric definitions"}
	definition.AddCommand(
		newDefinitionList(deps),
		newDefinitionInspect(deps),
		newDefinitionPull(deps),
		newDefinitionPublish(deps),
		newDefinitionCreate(deps),
		newDefinitionDelete(deps),
	)
	metric := &cobra.Command{Use: "metric", Short: "Manage Pulse metric variants and followers"}
	metric.AddCommand(
		newMetricList(deps),
		newMetricInspect(deps),
		newMetricFork(deps),
		newMetricDelete(deps),
		newMetricFollowers(deps),
		newMetricFollow(deps),
		newMetricUnfollow(deps),
	)
	command.AddCommand(definition, metric)
	return command
}

func newDefinitionDelete(deps Dependencies) *cobra.Command {
	var input definitiondelete.Input
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

func newMetricDelete(deps Dependencies) *cobra.Command {
	var input metricdelete.Input
	command := exactIDCommand("delete", "Delete one exact Pulse metric.", "pulse.metric.delete", &input.LUID, func(command *cobra.Command) error {
		if input.Environment == "" {
			return usage("pulse.metric.delete", "--environment is required")
		}
		result, err := deps.MetricDeleter.DeletePulseMetric(command.Context(), input)
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
	var input definitionlist.Input
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
	return command
}

func newDefinitionInspect(deps Dependencies) *cobra.Command {
	var input definitioninspect.Input
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
	var input definitionpull.Input
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
	var input definitionpublish.Input
	command := actionCommand("publish", "Recreate a portable Pulse bundle as new definitions and metrics.", "pulse.definition.publish", func(command *cobra.Command) error {
		if err := definitionpublish.ValidateInput(input); err != nil {
			return err
		}
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
	var input definitioncreate.Input
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

func newMetricList(deps Dependencies) *cobra.Command {
	var input metriclist.Input
	command := &cobra.Command{
		Use:         "list",
		Short:       "List one definition's Pulse metrics.",
		Annotations: capability("pulse.metric.list"),
		Args: func(command *cobra.Command, args []string) error {
			if err := noArgs("pulse.metric.list", command, args); err != nil {
				return err
			}
			if input.DefinitionLUID == "" {
				return usage("pulse.metric.list", "--definition-id is required")
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.MetricLister.ListPulseMetrics(command.Context(), input)
			if err != nil {
				return clierr.WithOutput(result, err)
			}
			return deps.Renderer.Render(result)
		},
	}
	readFlags(command, &input.Environment, &input.Cache)
	command.Flags().StringVar(&input.DefinitionLUID, "definition-id", "", "authoritative Pulse definition LUID")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum metrics to return from 1 through 10000; defaults to 25")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	_ = command.Flags().MarkHidden("cursor")
	command.Flags().BoolVar(&input.All, "all", false, "return all metrics within 100 pages and 10,000 records; cannot combine with --limit")
	return command
}

func newMetricInspect(deps Dependencies) *cobra.Command {
	var input metricinspect.Input
	command := exactIDCommand("inspect", "Inspect one exact Pulse metric.", "pulse.metric.inspect", &input.LUID, func(command *cobra.Command) error {
		result, err := deps.MetricInspector.InspectPulseMetric(command.Context(), input)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	readFlags(command, &input.Environment, &input.Cache)
	return command
}

func newMetricFork(deps Dependencies) *cobra.Command {
	var input metricfork.Input
	var includeFilters, excludeFilters []string
	var preview bool
	command := &cobra.Command{
		Use:   "fork",
		Short: "Create one Pulse metric variant.",
		Long: "Create one Pulse metric variant from an existing metric.\n\n" +
			"Repeat --filter '<field>=<value>' to add values or fields. Use --exclude-filter for excluded values.",
		Annotations: capability("pulse.metric.fork"),
		Args: func(command *cobra.Command, args []string) error {
			if err := noArgs("pulse.metric.fork", command, args); err != nil {
				return err
			}
			if input.Environment == "" || input.MetricLUID == "" {
				return usage("pulse.metric.fork", "--environment and --id are required")
			}
			filters, err := parseFilters(includeFilters, excludeFilters)
			if err != nil {
				return clierr.Usage("pulse.metric.fork", err)
			}
			input.Filters = filters
			if input.Timeframe == "" && len(input.Filters) == 0 {
				return usage("pulse.metric.fork", "at least one of --period, --filter, or --exclude-filter is required")
			}
			period := strings.ToUpper(strings.TrimSpace(input.Timeframe))
			if input.CustomDays != 0 && period != "CUSTOM_N_DAYS" {
				return usage("pulse.metric.fork", "--days requires --period CUSTOM_N_DAYS")
			}
			if period == "CUSTOM_N_DAYS" && input.CustomDays == 0 {
				return usage("pulse.metric.fork", "--period CUSTOM_N_DAYS requires --days")
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.MetricForker.ForkPulseMetric(command.Context(), input, preview)
			if err != nil {
				return clierr.WithOutput(result, err)
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&input.MetricLUID, "id", "", "authoritative source metric LUID")
	command.Flags().StringVar(&input.Timeframe, "period", "", "timeframe such as LAST_30_DAYS, MONTH_TO_DATE, or CUSTOM_N_DAYS")
	command.Flags().IntVar(&input.CustomDays, "days", 0, "custom trailing day count from 1 through 3650")
	command.Flags().StringArrayVar(&includeFilters, "filter", nil, "included dimensional value as <field ID or unique display name>=<value>; repeat for more values or fields")
	command.Flags().StringArrayVar(&excludeFilters, "exclude-filter", nil, "excluded dimensional value as <field ID or unique display name>=<value>; repeat for more values or fields")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func newMetricFollowers(deps Dependencies) *cobra.Command {
	var input metricfollowers.Input
	command := exactIDCommand("followers", "List one Pulse metric's followers.", "pulse.metric.followers", &input.MetricLUID, func(command *cobra.Command) error {
		result, err := deps.MetricFollowers.ListPulseMetricFollowers(command.Context(), input)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	readFlags(command, &input.Environment, &input.Cache)
	return command
}

func newMetricFollow(deps Dependencies) *cobra.Command {
	var input metricfollow.Input
	var preview bool
	command := &cobra.Command{
		Use:         "follow",
		Short:       "Add one Pulse metric follower.",
		Annotations: capability("pulse.metric.follow"),
		Args: func(command *cobra.Command, args []string) error {
			if err := noArgs("pulse.metric.follow", command, args); err != nil {
				return err
			}
			if input.Environment == "" || input.MetricLUID == "" || (input.UserLUID == "") == (input.GroupLUID == "") {
				return usage("pulse.metric.follow", "--environment, --id, and exactly one of --user-id or --group-id are required")
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.MetricFollower.FollowPulseMetric(command.Context(), input, preview)
			if err != nil {
				return clierr.WithOutput(result, err)
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&input.MetricLUID, "id", "", "authoritative metric LUID")
	command.Flags().StringVar(&input.UserLUID, "user-id", "", "authoritative follower user LUID")
	command.Flags().StringVar(&input.GroupLUID, "group-id", "", "authoritative follower group LUID")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func newMetricUnfollow(deps Dependencies) *cobra.Command {
	var input metricunfollow.Input
	var preview bool
	command := &cobra.Command{
		Use:   "unfollow",
		Short: "Remove one Pulse metric follower.",
		Long: "Remove one Pulse metric follower.\n\n" +
			"Use --subscription-id to remove one known subscription.\n" +
			"Otherwise, use --id with exactly one of --user-id or --group-id to resolve one subscription.",
		Annotations: capability("pulse.metric.unfollow"),
		Args: func(command *cobra.Command, args []string) error {
			if err := noArgs("pulse.metric.unfollow", command, args); err != nil {
				return err
			}
			if input.Environment == "" {
				return usage("pulse.metric.unfollow", "--environment is required")
			}
			direct := input.SubscriptionLUID != "" && input.MetricLUID == "" && input.UserLUID == "" && input.GroupLUID == ""
			relation := input.SubscriptionLUID == "" && input.MetricLUID != "" && (input.UserLUID != "") != (input.GroupLUID != "")
			if !direct && !relation {
				return usage("pulse.metric.unfollow", "use either --subscription-id or --id with exactly one of --user-id or --group-id")
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.MetricUnfollower.UnfollowPulseMetric(command.Context(), input, preview)
			if err != nil {
				return clierr.WithOutput(result, err)
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&input.SubscriptionLUID, "subscription-id", "", "authoritative subscription LUID; cannot be combined with other selectors")
	command.Flags().StringVar(&input.MetricLUID, "id", "", "authoritative metric LUID; requires exactly one follower selector")
	command.Flags().StringVar(&input.UserLUID, "user-id", "", "authoritative user follower LUID; requires --id")
	command.Flags().StringVar(&input.GroupLUID, "group-id", "", "authoritative group follower LUID; requires --id")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func actionCommand(use, short, operation string, run func(*cobra.Command) error) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Annotations: capability(operation),
		Args: func(command *cobra.Command, args []string) error {
			return noArgs(operation, command, args)
		},
		RunE: func(command *cobra.Command, _ []string) error { return run(command) },
	}
}

func exactIDCommand(use, short, operation string, target *string, run func(*cobra.Command) error) *cobra.Command {
	command := &cobra.Command{
		Use:         use,
		Short:       short,
		Annotations: capability(operation),
		Args: func(command *cobra.Command, args []string) error {
			if err := noArgs(operation, command, args); err != nil {
				return err
			}
			if strings.TrimSpace(*target) == "" {
				return usage(operation, "--id is required")
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error { return run(command) },
	}
	command.Flags().StringVar(target, "id", "", "authoritative Pulse resource LUID")
	return command
}

func readFlags(command *cobra.Command, environment *string, cache *bool) {
	command.Flags().StringVar(environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().BoolVar(cache, "cache", false, "read indexed local cache data without contacting Tableau")
}

func capability(operation string) map[string]string {
	return map[string]string{"tadx.capability": operation}
}

func noArgs(operation string, command *cobra.Command, args []string) error {
	if err := cobra.NoArgs(command, args); err != nil {
		return clierr.Usage(operation, err)
	}
	return nil
}

func usage(operation, message string) error {
	return clierr.Usage(operation, errors.New(message))
}

type filterKey struct {
	field   string
	exclude bool
}

func parseFilters(included, excluded []string) ([]metricfork.Filter, error) {
	grouped := make(map[filterKey][]string, len(included)+len(excluded))
	modes := make(map[string]bool, len(included)+len(excluded))
	for _, group := range []struct {
		values  []string
		exclude bool
	}{{included, false}, {excluded, true}} {
		for _, raw := range group.values {
			field, value, ok := strings.Cut(raw, "=")
			field = strings.TrimSpace(field)
			if !ok || field == "" || value == "" {
				return nil, fmt.Errorf("filter %q must use <field>=<value>", raw)
			}
			if mode, exists := modes[field]; exists && mode != group.exclude {
				return nil, fmt.Errorf("field %q cannot have both included and excluded values", field)
			}
			modes[field] = group.exclude
			key := filterKey{field: field, exclude: group.exclude}
			grouped[key] = append(grouped[key], value)
		}
	}
	filters := make([]metricfork.Filter, 0, len(grouped))
	for key, values := range grouped {
		sort.Strings(values)
		filters = append(filters, metricfork.Filter{Field: key.field, Values: values, Exclude: key.exclude})
	}
	sort.Slice(filters, func(i, j int) bool { return filters[i].Field < filters[j].Field })
	return filters, nil
}
