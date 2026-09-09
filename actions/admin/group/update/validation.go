package update

import (
	"strings"
)

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.GroupLUID) == "" {
		return usage("selector", "admin group update requires explicit environment and group LUID")
	}
	if in.Name == nil && in.MinimumSiteRole == nil && in.ExternalUserEnabled == nil && !in.MembershipSet {
		return usage("fields", "admin group update requires metadata or an explicit desired membership")
	}
	_, err := normalizeDesired(in.DesiredMemberLUIDs, in.MembershipSet)
	return err
}
