package identity

import (
	"fmt"
	"strings"
	"unicode"
)

// ValidateLUIDShape validates only the local syntax of an opaque Tableau LUID.
// It deliberately does not assume UUID formatting or prove remote existence.
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
