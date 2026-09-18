package unfollow

import (
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pulsecontract"
)

// ValidateInput checks subscription-or-relationship selection without remote reads.
func ValidateInput(input Input) error {
	subscription, metric, user, group := strings.TrimSpace(input.SubscriptionLUID), strings.TrimSpace(input.MetricLUID), strings.TrimSpace(input.UserLUID), strings.TrimSpace(input.GroupLUID)
	direct := subscription != "" && metric == "" && user == "" && group == ""
	relation := subscription == "" && metric != "" && (user != "") != (group != "")
	if !direct && !relation {
		return fail("pulse.metric.unfollow.usage", errs.KindUsage, input, "Use either one exact subscription LUID or one exact metric and follower pair.", nil)
	}
	if relation {
		if err := pulsecontract.ValidateLUIDShape("metric", input.MetricLUID); err != nil {
			return fail("pulse.metric.unfollow.usage", errs.KindUsage, input, "Pulse metric unfollow requires a well-formed exact metric LUID.", err)
		}
		if user != "" {
			if err := pulsecontract.ValidateLUIDShape("user", input.UserLUID); err != nil {
				return fail("pulse.metric.unfollow.usage", errs.KindUsage, input, "Pulse metric unfollow requires a well-formed exact user LUID.", err)
			}
		}
	}
	return nil
}
