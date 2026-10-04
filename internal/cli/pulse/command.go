// Package pulse contains thin Cobra plumbing for Tableau Pulse lifecycle commands.
package pulse

import (
	"context"
	"errors"
	"strings"

	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
	subscriptionlist "github.com/ahillspace/tadx/actions/pulse/subscription"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// Renderer writes one structured result.
type Renderer interface{ Render(any) error }

// DefinitionLister lists Pulse definitions.
type DefinitionLister interface {
	ListPulseDefinitions(context.Context, pulsedefinition.ListInput) (pulsedefinition.ListOutput, error)
}

// DefinitionInspector inspects one Pulse definition.
type DefinitionInspector interface {
	InspectPulseDefinition(context.Context, pulsedefinition.InspectInput) (pulsedefinition.InspectOutput, error)
}

// DefinitionPuller pulls one Pulse definition artifact.
type DefinitionPuller interface {
	PullPulseDefinition(context.Context, pulsedefinition.PullInput) (pulsedefinition.PullOutput, error)
}

type DefinitionPublisher interface {
	PublishPulseDefinition(context.Context, pulsedefinition.PublishInput) (pulsedefinition.PublishOutput, error)
}

// DefinitionCreator creates one Pulse definition or returns a preview.
type DefinitionCreator interface {
	CreatePulseDefinition(context.Context, pulsedefinition.CreateInput, bool) (pulsedefinition.CreateOutput, error)
}

// DefinitionDeleter deletes one Pulse definition or returns a preview.
type DefinitionDeleter interface {
	DeletePulseDefinition(context.Context, pulsedefinition.DeleteInput) (pulsedefinition.DeleteOutput, error)
}

// MetricLister lists the metrics for one definition.
type MetricLister interface {
	ListPulseMetrics(context.Context, pulsemetric.ListInput) (pulsemetric.ListOutput, error)
}

// MetricInspector inspects one Pulse metric.
type MetricInspector interface {
	InspectPulseMetric(context.Context, pulsemetric.InspectInput) (pulsemetric.InspectOutput, error)
}

// MetricForker creates one Pulse metric variant or returns a preview.
type MetricForker interface {
	ForkPulseMetric(context.Context, pulsemetric.ForkInput, bool) (pulsemetric.ForkOutput, error)
}

// MetricDeleter deletes one Pulse metric or returns a preview.
type MetricDeleter interface {
	DeletePulseMetric(context.Context, pulsemetric.DeleteInput) (pulsemetric.DeleteOutput, error)
}

// MetricFollowers lists one metric's subscriptions.
type MetricFollowers interface {
	ListPulseMetricFollowers(context.Context, pulsemetric.FollowersInput) (pulsemetric.FollowersOutput, error)
}

// MetricFollower creates one Pulse subscription or returns a preview.
type MetricFollower interface {
	FollowPulseMetric(context.Context, pulsemetric.FollowInput, bool) (pulsemetric.FollowOutput, error)
}

// MetricUnfollower removes one Pulse subscription or returns a preview.
type MetricUnfollower interface {
	UnfollowPulseMetric(context.Context, pulsemetric.UnfollowInput, bool) (pulsemetric.UnfollowOutput, error)
}

// SubscriptionLister lists the authenticated user's Pulse subscriptions.
type SubscriptionLister interface {
	ListPulseSubscriptions(context.Context, subscriptionlist.Input) (subscriptionlist.Output, error)
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
	SubscriptionLister  SubscriptionLister
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
	subscription := &cobra.Command{Use: "subscription", Short: "Inspect your Pulse subscriptions"}
	subscription.AddCommand(newSubscriptionList(deps))
	command.AddCommand(definition, metric, subscription)
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
