package value

import "testing"

func TestSuffixedContentURLMatchesOnlyTableauCollisionSuffixes(t *testing.T) {
	for _, test := range []struct {
		contentURL, candidate string
		want                  bool
	}{
		{"SalesMargin", "SalesMargin_1", true},
		{"SalesMargin", "SalesMargin_17932075825460", true},
		{"SalesMargin", "SalesMargin", false},
		{"SalesMargin", "SalesMargin_", false},
		{"SalesMargin", "SalesMargin_x1", false},
		{"SalesMargin", "SalesMargin_1x", false},
		{"SalesMargin", "SalesMarginX_1", false},
		{"SalesMargin", "salesmargin_1", false},
		{"", "_1", false},
	} {
		if got := SuffixedContentURL(test.contentURL, test.candidate); got != test.want {
			t.Errorf("SuffixedContentURL(%q, %q) = %v, want %v", test.contentURL, test.candidate, got, test.want)
		}
	}
}
