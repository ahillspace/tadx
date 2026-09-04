// Package pulse contains thin Cobra plumbing for Tableau Pulse lifecycle commands.
package pulse

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	definitioncreate "github.com/ahillspace/tadx/actions/pulse/definition/create"
	definitionget "github.com/ahillspace/tadx/actions/pulse/definition/get"
	definitionlist "github.com/ahillspace/tadx/actions/pulse/definition/list"
	definitionpull "github.com/ahillspace/tadx/actions/pulse/definition/pull"
	metricfollow "github.com/ahillspace/tadx/actions/pulse/metric/follow"
	metricfollowers "github.com/ahillspace/tadx/actions/pulse/metric/followers"
	metricfork "github.com/ahillspace/tadx/actions/pulse/metric/fork"
	metricget "github.com/ahillspace/tadx/actions/pulse/metric/get"
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

// DefinitionGetter gets one Pulse definition.
type DefinitionGetter interface {
	GetPulseDefinition(context.Context, definitionget.Input) (definitionget.Output, error)
}

// DefinitionPuller pulls one Pulse definition artifact.
type DefinitionPuller interface {
	PullPulseDefinition(context.Context, definitionpull.Input) (definitionpull.Output, error)
}

// DefinitionCreator previews or creates one Pulse definition.
type DefinitionCreator interface {
	CreatePulseDefinition(context.Context, definitioncreate.Input, bool) (definitioncreate.Output, error)
}

// MetricLister lists the metrics for one definition.
type MetricLister interface {
	ListPulseMetrics(context.Context, metriclist.Input) (metriclist.Output, error)
}

// MetricGetter gets one Pulse metric.
type MetricGetter interface {
	GetPulseMetric(context.Context, metricget.Input) (metricget.Output, error)
}

// MetricForker previews or creates one Pulse metric variant.
type MetricForker interface {
	ForkPulseMetric(context.Context, metricfork.Input, bool) (metricfork.Output, error)
}

// MetricFollowers lists one metric's subscriptions.
type MetricFollowers interface {
	ListPulseMetricFollowers(context.Context, metricfollowers.Input) (metricfollowers.Output, error)
}

// MetricFollower previews or creates one Pulse subscription.
type MetricFollower interface {
	FollowPulseMetric(context.Context, metricfollow.Input, bool) (metricfollow.Output, error)
}

// MetricUnfollower previews or removes one Pulse subscription.
type MetricUnfollower interface {
	UnfollowPulseMetric(context.Context, metricunfollow.Input, bool) (metricunfollow.Output, error)
}

// Dependencies contains Pulse command wiring.
type Dependencies struct {
	DefinitionLister  DefinitionLister
	DefinitionGetter  DefinitionGetter
	DefinitionPuller  DefinitionPuller
	DefinitionCreator DefinitionCreator
	MetricLister      MetricLister
	MetricGetter      MetricGetter
	MetricForker      MetricForker
	MetricFollowers   MetricFollowers
	MetricFollower    MetricFollower
	MetricUnfollower  MetricUnfollower
	Renderer          Renderer
}

// New creates the Pulse command tree.
func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "pulse",
		Short: "Manage Tableau Pulse definitions and metrics",
		Long: "Manage Tableau Pulse definitions, metric variants, and followers.\n\n" +
			"Discover source fields with tadx content datasource schema --id <datasource-luid> --query <term> before creating a definition.",
	}
	definition := &cobra.Command{Use: "definition", Short: "Manage Pulse metric definitions"}
	definition.AddCommand(
		newDefinitionList(deps),
		newDefinitionGet(deps),
		newDefinitionPull(deps),
		newDefinitionCreate(deps),
	)
	metric := &cobra.Command{Use: "metric", Short: "Manage Pulse metric variants and followers"}
	metric.AddCommand(
		newMetricList(deps),
		newMetricGet(deps),
		newMetricFork(deps),
		newMetricFollowers(deps),
		newMetricFollow(deps),
		newMetricUnfollow(deps),
	)
	command.AddCommand(definition, metric)
	return command
}

func newDefinitionList(deps Dependencies) *cobra.Command {
	var input definitionlist.Input
	command := actionCommand("list", "List one bounded Pulse definition page.", "pulse.definition.list", func(command *cobra.Command) error {
		result, err := deps.DefinitionLister.ListPulseDefinitions(command.Context(), input)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(result)
	})
	readFlags(command, &input.Environment, &input.Catalog)
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum definitions to return; defaults to 25")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	return command
}

func newDefinitionGet(deps Dependencies) *cobra.Command {
	var input definitionget.Input
	command := exactIDCommand("get", "Inspect one exact Pulse definition.", "pulse.definition.get", &input.LUID, func(command *cobra.Command) error {
		result, err := deps.DefinitionGetter.GetPulseDefinition(command.Context(), input)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(result)
	})
	readFlags(command, &input.Environment, &input.Catalog)
	return command
}

func newDefinitionPull(deps Dependencies) *cobra.Command {
	var input definitionpull.Input
	command := exactIDCommand("pull", "Pull one Pulse definition artifact.", "pulse.definition.pull", &input.LUID, func(command *cobra.Command) error {
		result, err := deps.DefinitionPuller.PullPulseDefinition(command.Context(), input)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(result)
	})
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	command.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace a dirty local artifact")
	return command
}

func newDefinitionCreate(deps Dependencies) *cobra.Command {
	var input definitioncreate.Input
	var apply bool
	command := &cobra.Command{
		Use:         "create",
		Short:       "Preview or create one Pulse definition.",
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
			result, err := deps.DefinitionCreator.CreatePulseDefinition(command.Context(), input, apply)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&input.Intent.Name, "name", "", "definition name")
	command.Flags().StringVar(&input.Intent.Description, "description", "", "definition description")
	command.Flags().StringVar(&input.Intent.DatasourceLUID, "datasource-id", "", "authoritative published datasource LUID")
	command.Flags().StringVar(&input.Intent.MeasureField, "measure-field", "", "exact raw Tableau measure field ID")
	command.Flags().StringVar(&input.Intent.Aggregation, "aggregation", "", "aggregation: SUM, AVERAGE, MIN, MAX, COUNT, COUNT_DISTINCT, or USER; defaults to SUM")
	command.Flags().StringVar(&input.Intent.TimeDimension, "date-field", "", "exact raw Tableau date field ID")
	command.Flags().StringArrayVar(&input.Intent.AllowedDimensions, "dimension", nil, "exact raw Tableau dimension field ID; repeat for each allowed dimension")
	command.Flags().StringVar(&input.Intent.MinimumGranularity, "minimum-granularity", "", "minimum date granularity: DAY, WEEK, MONTH, QUARTER, or YEAR; defaults to DAY")
	command.Flags().StringVar(&input.Intent.NumberFormat, "number-format", "", "number format: NUMBER, CURRENCY, or PERCENT; defaults to NUMBER")
	command.Flags().StringVar(&input.Intent.CurrencyCode, "currency", "", "three-letter currency code when --number-format is CURRENCY; defaults to USD")
	command.Flags().StringVar(&input.Intent.Sentiment, "sentiment", "", "sentiment: UP, DOWN, or NONE; defaults to NONE")
	command.Flags().StringVar(&input.Intent.Temporality, "temporality", "", "temporality: OVER_TIME or LATEST; defaults to OVER_TIME")
	command.Flags().BoolVar(&input.Intent.RunningTotal, "running-total", false, "create a running total; requires SUM and OVER_TIME")
	command.Flags().BoolVar(&apply, "apply", false, "apply the previewed remote mutation")
	return command
}

func newMetricList(deps Dependencies) *cobra.Command {
	var input metriclist.Input
	command := &cobra.Command{
		Use:         "list",
		Short:       "List one definition's bounded Pulse metric page.",
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
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	readFlags(command, &input.Environment, &input.Catalog)
	command.Flags().StringVar(&input.DefinitionLUID, "definition-id", "", "authoritative Pulse definition LUID")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum metrics to return; defaults to 25")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	return command
}

func newMetricGet(deps Dependencies) *cobra.Command {
	var input metricget.Input
	command := exactIDCommand("get", "Inspect one exact Pulse metric.", "pulse.metric.get", &input.LUID, func(command *cobra.Command) error {
		result, err := deps.MetricGetter.GetPulseMetric(command.Context(), input)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(result)
	})
	readFlags(command, &input.Environment, &input.Catalog)
	return command
}

func newMetricFork(deps Dependencies) *cobra.Command {
	var input metricfork.Input
	var includeFilters, excludeFilters []string
	var apply bool
	command := &cobra.Command{
		Use:   "fork",
		Short: "Preview or create one Pulse metric variant.",
		Long: "Preview or create one Pulse metric variant from an existing metric.\n\n" +
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
			result, err := deps.MetricForker.ForkPulseMetric(command.Context(), input, apply)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&input.MetricLUID, "id", "", "authoritative source metric LUID")
	command.Flags().StringVar(&input.Timeframe, "period", "", "timeframe such as LAST_30_DAYS, MONTH_TO_DATE, or CUSTOM_N_DAYS")
	command.Flags().IntVar(&input.CustomDays, "days", 0, "custom trailing day count from 1 through 3650")
	command.Flags().StringArrayVar(&includeFilters, "filter", nil, "included dimensional value as <field>=<value>; repeat for more values or fields")
	command.Flags().StringArrayVar(&excludeFilters, "exclude-filter", nil, "excluded dimensional value as <field>=<value>; repeat for more values or fields")
	command.Flags().BoolVar(&apply, "apply", false, "apply the previewed remote mutation")
	return command
}

func newMetricFollowers(deps Dependencies) *cobra.Command {
	var input metricfollowers.Input
	command := exactIDCommand("followers", "List one Pulse metric's followers.", "pulse.metric.followers", &input.MetricLUID, func(command *cobra.Command) error {
		result, err := deps.MetricFollowers.ListPulseMetricFollowers(command.Context(), input)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(result)
	})
	readFlags(command, &input.Environment, &input.Catalog)
	return command
}

func newMetricFollow(deps Dependencies) *cobra.Command {
	var input metricfollow.Input
	var apply bool
	command := &cobra.Command{
		Use:         "follow",
		Short:       "Preview or add one Pulse metric follower.",
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
			result, err := deps.MetricFollower.FollowPulseMetric(command.Context(), input, apply)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&input.MetricLUID, "id", "", "authoritative metric LUID")
	command.Flags().StringVar(&input.UserLUID, "user-id", "", "authoritative follower user LUID")
	command.Flags().StringVar(&input.GroupLUID, "group-id", "", "authoritative follower group LUID")
	command.Flags().BoolVar(&apply, "apply", false, "apply the previewed remote mutation")
	return command
}

func newMetricUnfollow(deps Dependencies) *cobra.Command {
	var input metricunfollow.Input
	var apply bool
	command := &cobra.Command{
		Use:         "unfollow",
		Short:       "Preview or remove one Pulse metric follower.",
		Annotations: capability("pulse.metric.unfollow"),
		Args: func(command *cobra.Command, args []string) error {
			if err := noArgs("pulse.metric.unfollow", command, args); err != nil {
				return err
			}
			if input.Environment == "" {
				return usage("pulse.metric.unfollow", "--environment is required")
			}
			direct := input.SubscriptionLUID != ""
			relation := input.MetricLUID != "" && (input.UserLUID != "") != (input.GroupLUID != "")
			if direct == relation {
				return usage("pulse.metric.unfollow", "use either --subscription-id or --id with exactly one of --user-id or --group-id")
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.MetricUnfollower.UnfollowPulseMetric(command.Context(), input, apply)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&input.SubscriptionLUID, "subscription-id", "", "authoritative Pulse subscription LUID")
	command.Flags().StringVar(&input.MetricLUID, "id", "", "authoritative metric LUID for exact follower resolution")
	command.Flags().StringVar(&input.UserLUID, "user-id", "", "authoritative follower user LUID")
	command.Flags().StringVar(&input.GroupLUID, "group-id", "", "authoritative follower group LUID")
	command.Flags().BoolVar(&apply, "apply", false, "apply the previewed remote mutation")
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

func readFlags(command *cobra.Command, environment *string, catalog *bool) {
	command.Flags().StringVar(environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().BoolVar(catalog, "catalog", false, "read indexed local catalog data without contacting Tableau")
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
