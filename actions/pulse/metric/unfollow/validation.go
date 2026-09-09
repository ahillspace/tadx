package unfollow

import (
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// ValidateInput checks subscription-or-relationship selection without remote reads.
func ValidateInput(input Input) error {
	subscription, metric, user, group := strings.TrimSpace(input.SubscriptionLUID), strings.TrimSpace(input.MetricLUID), strings.TrimSpace(input.UserLUID), strings.TrimSpace(input.GroupLUID)
	direct := subscription != "" && metric == "" && user == "" && group == ""
	relation := subscription == "" && metric != "" && (user != "") != (group != "")
	if !direct && !relation {
		return fail("pulse.metric.unfollow.usage", errs.KindUsage, input, "Use either one exact subscription LUID or one exact metric and follower pair.", nil)
	}
	return nil
}
