package pulsecontract

import (
	"fmt"
	"strings"
	"unicode"
)

// ValidateLUIDShape checks the local syntax of an opaque Tableau LUID.
// Pulse LUIDs are authoritative opaque values, so this deliberately does not
// impose a UUID-only rule that would reject valid provider identifiers.
func ValidateLUIDShape(kind, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s LUID %q must be a non-empty token without whitespace", kind, value)
	}
	if len([]rune(trimmed)) > 255 {
		return fmt.Errorf("%s LUID %q must be at most 255 characters", kind, value)
	}
	for _, char := range trimmed {
		if unicode.IsSpace(char) || unicode.IsControl(char) {
			return fmt.Errorf("%s LUID %q must be a non-empty token without whitespace", kind, value)
		}
	}
	return nil
}
