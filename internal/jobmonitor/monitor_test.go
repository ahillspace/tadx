package jobmonitor

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/value"
)

func receipt(id string, now time.Time, bulk bool) Receipt {
	pool := now.Add(time.Minute)
	if bulk {
		pool = now
	}
	return Receipt{Version: 1, Operation: "workbook.publish", Environment: "dev", Server: "https://example.test", SiteID: "site", CoordinationKey: "opaque-key", AcceptedAt: now, PoolAfter: pool, Observation: value.JobStatus{ID: id, Type: "PublishWorkbook", Status: "pending"}}
}

func TestPoolChecksActualRegisteredJobsTogether(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	store := Store{Directory: t.TempDir()}
	first, second := receipt("job-a", now, true), receipt("job-b", now, true)
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

func TestSingleTransitionsAfterMinuteAndContinuesBeyondTenMinutes(t *testing.T) {
	start := time.Unix(1_800_000_000, 0).UTC()
	now := start
	store := Store{Directory: t.TempDir()}
	single, other := receipt("single", start, false), receipt("other", start, true)
	for _, r := range []Receipt{single, other} {
		if _, err := store.Register(t.Context(), r); err != nil {
			t.Fatal(err)
		}
	}
	fast, pooled := 0, 0
	monitor := Monitor{Store: store, Now: func() time.Time { return now }, Sleep: func(_ context.Context, d time.Duration) error { now = now.Add(d); return nil }, Observe: func(_ context.Context, jobs []Receipt) []CheckResult {
		if now.Sub(start) < time.Minute {
			fast++
			if len(jobs) != 1 || jobs[0].Observation.ID != "single" {
				t.Fatal("single joined pool too early")
			}
		} else {
			pooled++
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
	if err != nil || got.Observation.Status != "succeeded" || fast == 0 || pooled == 0 || now.Sub(start) < 11*time.Minute {
		t.Fatalf("fast=%d pooled=%d duration=%s got=%+v err=%v", fast, pooled, now.Sub(start), got, err)
	}
}

func TestReadFailuresAndInterruptionNeverChangeRemoteJobToFailed(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	store := Store{Directory: t.TempDir()}
	r := receipt("pending", now, true)
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
