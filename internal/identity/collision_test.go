package identity

import (
	"strings"
	"testing"
)

func TestCollisionNameKeysUseUnicodeSimpleFolding(t *testing.T) {
	names := []string{"Destination", "destination", "Other", "\u03a3", "\u03c3", "\u03c2", "K", "k", "\u212a", "S", "s", "\u017f", "SS", "\u00df", "name ", "name"}
	for _, left := range names {
		for _, right := range names {
			if got, want := CollisionNameKey(left) == CollisionNameKey(right), strings.EqualFold(left, right); got != want {
				t.Errorf("%q / %q: key equality=%t EqualFold=%t", left, right, got, want)
			}
		}
	}
}
