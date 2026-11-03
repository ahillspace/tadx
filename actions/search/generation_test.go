package search_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ahillspace/tadx/actions/search"
)

func TestExpandedSearchGenerationRequiresAgreementAcrossEveryPage(t *testing.T) {
	first := search.Generation{ID: "generation-a", Environment: "fixture", Site: "fixture-site", GeneratedAt: "2026-11-03T12:00:00Z"}
	other := first
	other.ID = "generation-b"
	stale := first
	stale.Stale = true
	otherTarget := first
	otherTarget.Site = "another-site"
	otherTime := first
	otherTime.GeneratedAt = "2026-11-03T12:01:00Z"
	for _, test := range []struct {
		name        string
		generations []*search.Generation
		want        *search.Generation
		warnings    []string
	}{
		{"same generation", []*search.Generation{&first, &first}, &first, nil},
		{"different generation", []*search.Generation{&first, &other}, nil, nil},
		{"missing later generation", []*search.Generation{&first, nil}, nil, nil},
		{"missing first generation", []*search.Generation{nil, &first}, nil, nil},
		{"different then matching", []*search.Generation{&first, &other, &first}, nil, nil},
		{"missing then matching", []*search.Generation{&first, nil, &first}, nil, nil},
		{"different target", []*search.Generation{&first, &otherTarget}, nil, nil},
		{"different observation time", []*search.Generation{&first, &otherTime}, nil, nil},
		{"later staleness retained", []*search.Generation{&first, &stale}, &stale, nil},
		{"earlier staleness retained", []*search.Generation{&stale, &first}, &stale, nil},
		{"existing warning retained once", []*search.Generation{&first, &other}, nil, []string{search.UnsharedCacheGenerationWarning, search.UnsharedCacheGenerationWarning}},
		{"warning survives output bound", []*search.Generation{&first, &other}, nil, slices.Repeat([]string{"fixture diagnostic"}, 20)},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := make([]search.Generation, len(test.generations))
			pages := make([]search.Result, len(test.generations))
			for index, generation := range test.generations {
				if generation != nil {
					before[index] = *generation
				}
				pages[index] = search.Result{Source: "cache", Generation: generation, Items: []search.Item{{LUID: fmt.Sprintf("workbook-%d", index), Type: "workbook", Name: fmt.Sprintf("Workbook %d", index)}}}
				if index+1 < len(pages) {
					pages[index].Page.NextCursor = fmt.Sprintf("next-%d", index)
				}
			}
			pages[len(pages)-1].Warnings = test.warnings
			source := &pageSource{pages: pages}
			result, err := executeValidatedSearch(t.Context(), source, search.Input{Type: "workbook", Cache: true, Limit: 200})
			if err != nil || source.calls != len(pages) || len(result.Items) != len(pages) {
				t.Fatalf("complete action pages were not exercised: result=%+v error=%v calls=%d", result, err, source.calls)
			}
			if test.want == nil {
				if result.Generation != nil {
					t.Fatalf("aggregate claimed unsupported generation: %+v", result.Generation)
				}
				count := 0
				for _, warning := range result.Warnings {
					if warning == search.UnsharedCacheGenerationWarning {
						count++
					}
				}
				if count != 1 || len(result.Warnings) > 20 {
					t.Fatalf("aggregate omitted, duplicated, or overflowed the generation warning: %v", result.Warnings)
				}
			} else if result.Generation == nil || *result.Generation != *test.want {
				t.Fatalf("generation=%+v want%+v", result.Generation, test.want)
			}
			for index, generation := range test.generations {
				if generation != nil && (*generation != before[index] || result.Generation == generation) {
					t.Fatalf("aggregation changed or reused source-owned generation %d: %+v", index, generation)
				}
			}
		})
	}
}
