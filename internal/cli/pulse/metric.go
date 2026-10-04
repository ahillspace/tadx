package pulse

import (
	"fmt"
	"sort"
	"strings"

	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func newMetricDelete(deps Dependencies) *cobra.Command {
	var input pulsemetric.DeleteInput
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

func newMetricList(deps Dependencies) *cobra.Command {
	var input pulsemetric.ListInput
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
	command.MarkFlagsMutuallyExclusive("all", "limit")
	command.MarkFlagsMutuallyExclusive("all", "cursor")
	return command
}

func newMetricInspect(deps Dependencies) *cobra.Command {
	var input pulsemetric.InspectInput
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
	var input pulsemetric.ForkInput
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
			input.CustomDaysSet = command.Flags().Changed("days")
			if input.Timeframe == "" && len(input.Filters) == 0 {
				return usage("pulse.metric.fork", "at least one of --period, --filter, or --exclude-filter is required")
			}
			period := strings.ToUpper(strings.TrimSpace(input.Timeframe))
			if input.CustomDaysSet && period != "CUSTOM_N_DAYS" {
				return usage("pulse.metric.fork", "--days requires --period CUSTOM_N_DAYS")
			}
			if period == "CUSTOM_N_DAYS" && !input.CustomDaysSet {
				return usage("pulse.metric.fork", "--period CUSTOM_N_DAYS requires --days")
			}
			if input.CustomDaysSet && !pulsemetric.ForkIsSupportedCustomDays(input.CustomDays) {
				return usage("pulse.metric.fork", "--days must be one of 7, 14, 30, 60, or 90")
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
	command.Flags().IntVar(&input.CustomDays, "days", 0, "custom trailing day count: 7, 14, 30, 60, or 90")
	command.Flags().StringArrayVar(&includeFilters, "filter", nil, "included dimensional value as <field ID or unique display name>=<value>; repeat for more values or fields")
	command.Flags().StringArrayVar(&excludeFilters, "exclude-filter", nil, "excluded dimensional value as <field ID or unique display name>=<value>; repeat for more values or fields")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func newMetricFollowers(deps Dependencies) *cobra.Command {
	var input pulsemetric.FollowersInput
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
	var input pulsemetric.FollowInput
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
	var input pulsemetric.UnfollowInput
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

type filterKey struct {
	field   string
	exclude bool
}

func parseFilters(included, excluded []string) ([]pulsemetric.ForkFilter, error) {
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
	filters := make([]pulsemetric.ForkFilter, 0, len(grouped))
	for key, values := range grouped {
		sort.Strings(values)
		filters = append(filters, pulsemetric.ForkFilter{Field: key.field, Values: values, Exclude: key.exclude})
	}
	sort.Slice(filters, func(i, j int) bool { return filters[i].Field < filters[j].Field })
	return filters, nil
}
