package jobmonitor

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

func TestObserverReleasesCoordinationBetweenExactReads(t *testing.T) {
	var events []string
	observer := Observer{
		Open: func(context.Context) (ObservationSession, error) {
			events = append(events, "authenticate")
			return ObservationSession{Server: " HTTPS://TABLEAU.EXAMPLE/ ", SiteID: "site-1", Inspect: func(_ context.Context, id string) (value.JobStatus, error) {
				events = append(events, "inspect:"+id)
				return value.JobStatus{ID: id, Type: "refreshworkbook"}, nil
			}}, nil
		},
		Suspend: func(context.Context) error { events = append(events, "release"); return nil },
	}
	results := observer.Observe(t.Context(), []Receipt{
		{Server: "https://tableau.example", SiteID: "site-1", Observation: value.JobStatus{ID: "job-1", Type: "refreshworkbook"}},
		{Server: "https://tableau.example", SiteID: "site-1", Observation: value.JobStatus{ID: "job-2", Type: "refreshworkbook"}},
	})
	want := []string{"authenticate", "inspect:job-1", "release", "authenticate", "inspect:job-2", "release"}
	if !reflect.DeepEqual(events, want) || len(results) != 2 || results[0].Err != nil || results[1].Err != nil {
		t.Fatalf("events=%v results=%+v", events, results)
	}
}

func TestObserverRejectsTargetAndTypeChanges(t *testing.T) {
	for _, test := range []struct {
		name, server, site, kind, want string
		reads                          int
	}{
		{"server", "https://other.example", "site-1", "refreshworkbook", "target mismatch", 0},
		{"site", "https://tableau.example", "site-other", "refreshworkbook", "target mismatch", 0},
		{"type", "https://tableau.example", "site-1", "runflow", "changed the accepted job type", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			reads, releases := 0, 0
			observer := Observer{TargetError: "target mismatch",
				Open: func(context.Context) (ObservationSession, error) {
					return ObservationSession{Server: test.server, SiteID: test.site, Inspect: func(context.Context, string) (value.JobStatus, error) {
						reads++
						return value.JobStatus{ID: "job-1", Type: test.kind}, nil
					}}, nil
				},
				Suspend: func(context.Context) error { releases++; return nil },
			}
			result := observer.Observe(t.Context(), []Receipt{{Server: "https://tableau.example", SiteID: "site-1", Observation: value.JobStatus{ID: "job-1", Type: "refreshworkbook"}}})[0]
			if result.Err == nil || !strings.Contains(result.Err.Error(), test.want) || reads != test.reads || releases != 1 {
				t.Fatalf("result=%+v reads=%d releases=%d", result, reads, releases)
			}
		})
	}
}

func TestObserverReleasesAfterFailedAuthenticationDespiteCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	authErr, releaseErr := errors.New("authentication unavailable"), errors.New("release unavailable")
	releases := 0
	observer := Observer{
		Open: func(context.Context) (ObservationSession, error) { return ObservationSession{}, authErr },
		Suspend: func(ctx context.Context) error {
			releases++
			if ctx.Err() != nil {
				t.Fatal("release inherited cancellation")
			}
			return releaseErr
		},
	}
	result := observer.Observe(ctx, []Receipt{{Observation: value.JobStatus{ID: "job-1"}}})[0]
	if !errors.Is(result.Err, authErr) || !errors.Is(result.Err, releaseErr) || releases != 1 {
		t.Fatalf("result=%+v releases=%d", result, releases)
	}
}
