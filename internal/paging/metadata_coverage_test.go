package paging

import "testing"

func TestCountEvidence(t *testing.T) {
	for _, tc := range []struct {
		name           string
		total, count   int
		terminal, fail bool
	}{
		{"short-terminal", 3, 2, true, true},
		{"bounded-first-page", 3, 2, false, false},
		{"exact-terminal", 3, 3, true, false},
		{"growing-total", 4, 2, false, true},
		{"shrinking-total", 2, 2, true, true},
		{"over-count", 3, 4, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c MetadataCoverage
			if e := c.Page(3, 1, false); e != nil {
				t.Fatal(e)
			}
			if e := c.Page(tc.total, tc.count, tc.terminal); (e != nil) != tc.fail {
				t.Fatalf("error=%v want failure=%v", e, tc.fail)
			}
		})
	}
}
