package jobmonitor

import (
	"context"
	"errors"
	"testing"
)

func TestEligibilityPreservesSynchronousFallbackAndCommandMemo(t *testing.T) {
	var eligibility Eligibility
	calls := 0
	probe := func(context.Context) (bool, error) { calls++; return true, nil }
	denied := func() bool { return false }
	allowed := func() bool { return true }
	for _, tc := range []struct {
		kind   string
		policy func() bool
	}{
		{"flow", allowed},
		{"workbook", denied},
	} {
		got, err := eligibility.CanMonitor(t.Context(), tc.kind, "target", tc.policy, probe)
		if err != nil || got || calls != 0 {
			t.Fatalf("kind=%q eligible=%t calls=%d err=%v", tc.kind, got, calls, err)
		}
	}
	for range 2 {
		got, err := eligibility.CanMonitor(t.Context(), "workbook", "target", allowed, probe)
		if err != nil || !got || calls != 1 {
			t.Fatalf("eligible=%t calls=%d err=%v", got, calls, err)
		}
	}
	got, err := eligibility.CanMonitor(t.Context(), "workbook", "other", allowed, func(context.Context) (bool, error) {
		calls++
		return false, errors.New("role read failed")
	})
	if err != nil || got || calls != 2 {
		t.Fatalf("role failure eligible=%t calls=%d err=%v", got, calls, err)
	}
	got, err = eligibility.CanMonitor(t.Context(), "workbook", "other", allowed, probe)
	if err != nil || got || calls != 2 {
		t.Fatalf("memoized failure eligible=%t calls=%d err=%v", got, calls, err)
	}
}

func TestEligibilityCancellationDoesNotMemoizeRoleResult(t *testing.T) {
	var eligibility Eligibility
	ctx, cancel := context.WithCancel(t.Context())
	probe := func(context.Context) (bool, error) { cancel(); return true, nil }
	got, err := eligibility.CanMonitor(ctx, "datasource", "target", func() bool { return true }, probe)
	if got || !errors.Is(err, context.Canceled) {
		t.Fatalf("eligible=%t err=%v", got, err)
	}
	got, err = eligibility.CanMonitor(t.Context(), "datasource", "target", func() bool { return true }, func(context.Context) (bool, error) { return true, nil })
	if !got || err != nil {
		t.Fatalf("retry eligible=%t err=%v", got, err)
	}
}
