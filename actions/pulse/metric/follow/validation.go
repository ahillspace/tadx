package follow

import (
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pulsecontract"
)

// ValidateInput checks exact metric/follower selection before authentication.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.MetricLUID) == "" || (strings.TrimSpace(input.UserLUID) == "") == (strings.TrimSpace(input.GroupLUID) == "") {
		return fail("pulse.metric.follow.usage", errs.KindUsage, input, "Pulse metric follow requires an exact metric and exactly one user or group LUID.", nil)
	}
	if err := pulsecontract.ValidateLUIDShape("metric", input.MetricLUID); err != nil {
		return fail("pulse.metric.follow.usage", errs.KindUsage, input, "Pulse metric follow requires a well-formed exact metric LUID.", err)
	}
	if strings.TrimSpace(input.UserLUID) != "" {
		if err := pulsecontract.ValidateLUIDShape("user", input.UserLUID); err != nil {
			return fail("pulse.metric.follow.usage", errs.KindUsage, input, "Pulse metric follow requires a well-formed exact user LUID.", err)
		}
	}
	return nil
}
