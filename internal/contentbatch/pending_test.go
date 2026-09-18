package contentbatch

import (
	"context"
	"testing"
)

type pendingPublication struct{ State string }

func (p pendingPublication) OperationStatus() string { return p.State }

func TestAcceptedBatchIsNotReportedAsSucceeded(t *testing.T) {
	got, err := Run(t.Context(), "workbook.publish", []string{"one", "two"}, func(context.Context, string) (pendingPublication, error) {
		return pendingPublication{State: "pending"}, nil
	})
	if err != nil || got.Status != "running" || got.Succeeded != 0 {
		t.Fatalf("batch=%+v err=%v", got, err)
	}
	for _, item := range got.Items {
		if item.Status != "pending" {
			t.Errorf("item=%+v", item)
		}
	}
}

func TestInitialBatchObserverReportsAllQueuedItemsAsPending(t *testing.T) {
	var snapshots []Output
	ctx := WithObserver(t.Context(), func(out Output) {
		out.Items = append([]Item(nil), out.Items...)
		snapshots = append(snapshots, out)
	})

	got, err := Run(ctx, "workbook.publish", []string{"one", "two", "three"}, func(context.Context, string) (pendingPublication, error) {
		return pendingPublication{State: "pending"}, nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(snapshots) == 0 {
		t.Fatal("Run() did not notify the observer")
	}
	initial := snapshots[0]
	if initial.Status != "running" || initial.Total != 3 || initial.Pending != 3 || initial.Succeeded != 0 || initial.Failed != 0 || initial.Skipped != 0 {
		t.Fatalf("initial observer snapshot = %+v, want three pending items", initial)
	}
	for _, item := range initial.Items {
		if item.Status != "queued" {
			t.Errorf("initial queued item = %+v", item)
		}
	}
	if got.Pending != 3 || got.Status != "running" {
		t.Fatalf("final batch = %+v, want three pending items", got)
	}
}
