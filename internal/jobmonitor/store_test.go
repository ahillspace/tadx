package jobmonitor

import (
	"context"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/value"
)

func TestStaleReceiptCannotEraseConfirmedObservation(t *testing.T) {
	now := time.Now().UTC()
	store := Store{Directory: t.TempDir()}
	stale := receipt("confirmed-job", now, true)
	latest := stale
	latest.Observation = value.JobStatus{ID: stale.Observation.ID, Type: stale.Observation.Type, Status: "succeeded", ResourceID: "published-id", CheckedAt: now}
	latest.Verification = "confirmed"
	for _, item := range []Receipt{latest, stale} {
		if _, err := store.Save(t.Context(), item); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.Read(stale)
	if err != nil || got.Observation != latest.Observation || got.Verification != "confirmed" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestCancellationAfterObservationRetainsConfirmedResult(t *testing.T) {
	now := time.Now().UTC()
	store := Store{Directory: t.TempDir()}
	accepted := receipt("observed-job", now, true)
	if _, err := store.Register(t.Context(), accepted); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	monitor := Monitor{Store: store, Observe: func(context.Context, []Receipt) []CheckResult {
		cancel()
		return []CheckResult{{Status: value.JobStatus{ID: accepted.Observation.ID, Type: accepted.Observation.Type, Status: "succeeded", CheckedAt: now}}}
	}}
	got, err := monitor.Wait(ctx, accepted)
	if err != nil || got.Observation.Status != "succeeded" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	saved, err := store.Read(accepted)
	if err != nil || saved.Observation.Status != "succeeded" {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}
}
