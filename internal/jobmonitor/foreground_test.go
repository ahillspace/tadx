package jobmonitor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/value"
)

func TestForegroundCadenceDoesNotSlowBatches(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		for _, test := range []struct {
			age, interval time.Duration
		}{{0, 2 * time.Second}, {30 * time.Second, 5 * time.Second}, {10 * time.Minute, 15 * time.Second}} {
			start := time.Now().UTC()
			now := start.Add(test.age)
			store := Store{Directory: t.TempDir()}
			r := receipt("job", start, bulk)
			if _, err := store.Register(t.Context(), r); err != nil {
				t.Fatal(err)
			}
			checks := 0
			monitor := Monitor{Store: store, Now: func() time.Time { return now }, Sleep: func(_ context.Context, delay time.Duration) error {
				if delay != test.interval {
					t.Errorf("bulk=%v age=%v delay=%v want=%v", bulk, test.age, delay, test.interval)
				}
				now = now.Add(delay)
				return nil
			}, Observe: func(_ context.Context, jobs []Receipt) []CheckResult {
				checks++
				state := "running"
				if checks == 2 {
					state = "succeeded"
				}
				return []CheckResult{{Status: value.JobStatus{ID: "job", Status: state}}}
			}}
			if _, err := monitor.Wait(t.Context(), r); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestForegroundWaitLimitPreservesJobAndStopsSharedPolling(t *testing.T) {
	start := time.Now().UTC()
	now := start
	store := Store{Directory: t.TempDir()}
	r := receipt("unfinished", start, true)
	if _, err := store.Register(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	monitor := Monitor{Store: store, Now: func() time.Time { return now }, Sleep: func(_ context.Context, delay time.Duration) error { now = now.Add(delay); return nil }, Observe: func(_ context.Context, jobs []Receipt) []CheckResult {
		if now.Sub(start) >= 20*time.Minute {
			t.Fatal("polled beyond foreground wait budget")
		}
		return []CheckResult{{Status: value.JobStatus{ID: "unfinished", Status: "running"}}}
	}}
	got, err := monitor.Wait(t.Context(), r)
	if !errors.Is(err, ErrWaitLimit) || got.Observation.Status != "running" || now.Sub(start) != 20*time.Minute {
		t.Fatalf("duration=%v state=%s err=%v", now.Sub(start), got.Observation.Status, err)
	}
	other := receipt("other", now, true)
	if _, err := store.Register(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	monitor.Observe = func(_ context.Context, jobs []Receipt) []CheckResult {
		if len(jobs) != 1 || jobs[0].Observation.ID != "other" {
			t.Fatalf("expired job returned to shared polling: %+v", jobs)
		}
		return []CheckResult{{Status: value.JobStatus{ID: "other", Status: "succeeded"}}}
	}
	if _, err := monitor.Wait(t.Context(), other); err != nil {
		t.Fatal(err)
	}
}
