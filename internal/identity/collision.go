package identity

import (
	"strings"
	"unicode"
)

// CollisionNameKey identifies a case-insensitive creation or publication slot.
// It uses Unicode simple folding, the same equivalence as strings.EqualFold.
// Exact selectors and execution names retain their original spelling.
func CollisionNameKey(name string) string {
	return strings.Map(func(r rune) rune {
		key := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < key {
				key = next
			}
		}
		return key
	}, name)
}
