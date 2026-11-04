package search_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	tableausearch "github.com/ahillspace/tadx/internal/tableau/search"
)

func TestSearchAvailabilityAdviceMatchesRequestedResource(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		for _, scenario := range []struct {
			name  string
			types []string
			pulse bool
		}{
			{name: "content", types: []string{"workbook"}},
			{name: "Pulse metric", types: []string{"metric"}, pulse: true},
			{name: "mixed Cloud content", types: []string{"workbook", "metric"}, pulse: true},
		} {
			t.Run(http.StatusText(status)+"/"+scenario.name, func(t *testing.T) {
				calls := 0
				client := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					calls++
					writer.Header().Set("X-Tableau-Request-Id", "unavailable-search")
					writer.WriteHeader(status)
					_, _ = io.WriteString(writer, `{"message":"search unavailable"}`)
				}), "3.29")
				_, err := client.Search(t.Context(), tableausearch.Request{Terms: "Revenue", Types: scenario.types, Limit: 20})
				unavailable, ok := errors.AsType[*tableausearch.AvailabilityError](err)
				if !ok || !unavailable.NativeSearchUnavailable() || unavailable.Retryable() {
					t.Fatalf("unexpected availability error: %v", err)
				}
				upstream, ok := errors.AsType[*tableau.UpstreamError](err)
				if !ok || upstream.HTTPStatus() != status || tableau.RequestID(err) != "unavailable-search" || calls != 1 {
					t.Fatalf("lost upstream context or repeated request: calls=%d error=%v", calls, err)
				}
				advice := unavailable.CorrectiveAction()
				if scenario.pulse {
					if !strings.Contains(advice, "Tableau Cloud") || !strings.Contains(advice, "Pulse") || strings.Contains(advice, "Tableau Server") {
						t.Fatalf("Cloud-only Pulse search has invalid recovery advice: %q", advice)
					}
				} else if !strings.Contains(advice, "Tableau Server") || !strings.Contains(advice, "2022.3") {
					t.Fatalf("content search lost its valid Server version requirement: %q", advice)
				}
			})
		}
	}
}
