package jobmonitor

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/value"
)

func receipt(id string, now time.Time) Receipt {
	return Receipt{Version: 1, Operation: "workbook.publish", Environment: "dev", Server: "https://example.test", SiteID: "site", CoordinationKey: "opaque-key", AcceptedAt: now, Observation: value.JobStatus{ID: id, Type: "PublishWorkbook", Status: "pending"}}
}

func TestPoolChecksActualRegisteredJobsTogether(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	store := Store{Directory: t.TempDir()}
	first, second := receipt("job-a", now), receipt("job-b", now)
	for _, r := range []Receipt{first, second} {
		path, err := store.Register(t.Context(), r)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal("acceptance not durable before monitoring", err)
		}
	}
	requests, rounds := 0, 0
	monitor := Monitor{Store: store, Now: func() time.Time { return now }, Observe: func(_ context.Context, jobs []Receipt) []CheckResult {
		rounds++
		result := make([]CheckResult, len(jobs))
		for i, job := range jobs {
			requests++
			result[i].Status = value.JobStatus{ID: job.Observation.ID, Type: "PublishWorkbook", Status: "succeeded", CheckedAt: now}
		}
		return result
	}}
	for _, job := range []Receipt{first, second} {
		got, err := monitor.Wait(t.Context(), job)
		if err != nil || got.Observation.Status != "succeeded" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	}
	if rounds != 1 || requests != 2 {
		t.Fatalf("rounds=%d requests=%d", rounds, requests)
	}
}

func TestWaitPoolsRegisteredJobsAndContinuesBeyondTenMinutes(t *testing.T) {
	start := time.Unix(1_800_000_000, 0).UTC()
	now := start
	store := Store{Directory: t.TempDir()}
	single, other := receipt("single", start), receipt("other", start)
	for _, r := range []Receipt{single, other} {
		if _, err := store.Register(t.Context(), r); err != nil {
			t.Fatal(err)
		}
	}
	rounds := 0
	monitor := Monitor{Store: store, Now: func() time.Time { return now }, Sleep: func(_ context.Context, d time.Duration) error { now = now.Add(d); return nil }, Observe: func(_ context.Context, jobs []Receipt) []CheckResult {
		rounds++
		if len(jobs) != 2 {
			t.Fatalf("round %d observed %d jobs, want both registered jobs pooled", rounds, len(jobs))
		}
		out := make([]CheckResult, len(jobs))
		for i, job := range jobs {
			status := "running"
			if now.Sub(start) >= 11*time.Minute {
				status = "succeeded"
			}
			out[i].Status = value.JobStatus{ID: job.Observation.ID, Type: "PublishWorkbook", Status: status, CheckedAt: now}
		}
		return out
	}}
	got, err := monitor.Wait(t.Context(), single)
	if err != nil || got.Observation.Status != "succeeded" || rounds == 0 || now.Sub(start) < 11*time.Minute {
		t.Fatalf("rounds=%d duration=%s got=%+v err=%v", rounds, now.Sub(start), got, err)
	}
}

func TestReadFailuresAndInterruptionNeverChangeRemoteJobToFailed(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	store := Store{Directory: t.TempDir()}
	r := receipt("pending", now)
	if _, err := store.Register(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	monitor := Monitor{Store: store, Now: func() time.Time { return now }, Sleep: func(_ context.Context, d time.Duration) error { now = now.Add(d); return nil }, Observe: func(_ context.Context, jobs []Receipt) []CheckResult {
		return []CheckResult{{Err: errors.New("status unavailable")}}
	}}
	got, err := monitor.Wait(t.Context(), r)
	if err == nil || got.ReadFailures != 3 || got.Observation.Status != "pending" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err = monitor.Wait(ctx, r)
	if err == nil || got.Observation.Status != "pending" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
