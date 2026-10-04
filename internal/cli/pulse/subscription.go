package pulse

import (
	subscriptionlist "github.com/ahillspace/tadx/actions/pulse/subscription"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func newSubscriptionList(deps Dependencies) *cobra.Command {
	var input subscriptionlist.Input
	command := actionCommand("list", "List your Pulse subscriptions and saved metric configurations.", "pulse.subscription.list", func(command *cobra.Command) error {
		if command.Flags().Changed("limit") && (input.Limit < 1 || input.Limit > 10000) {
			return usage("pulse.subscription.list", "--limit must be between 1 and 10000")
		}
		result, err := deps.SubscriptionLister.ListPulseSubscriptions(command.Context(), input)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	command.Flags().StringVar(&input.Environment, "environment", "", "exact Tableau environment alias; defaults to the configured read environment")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum subscriptions to return from 1 through 10000; defaults to 25")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	_ = command.Flags().MarkHidden("cursor")
	command.Flags().BoolVar(&input.All, "all", false, "return all subscriptions within 100 pages and 10000 records; cannot combine with --limit")
	command.MarkFlagsMutuallyExclusive("all", "limit")
	command.MarkFlagsMutuallyExclusive("all", "cursor")
	return command
}
