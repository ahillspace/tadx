package follow

import (
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// ValidateInput checks exact metric/follower selection before authentication.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.MetricLUID) == "" || (strings.TrimSpace(input.UserLUID) == "") == (strings.TrimSpace(input.GroupLUID) == "") {
		return fail("pulse.metric.follow.usage", errs.KindUsage, input, "Pulse metric follow requires an exact metric and exactly one user or group LUID.", nil)
	}
	return nil
}
