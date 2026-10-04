package operationrun

import (
	"context"
	"testing"
	"time"
)

func TestPublicationExecutionObservationAndIntentBoundaries(t *testing.T) {
	var absent *PublicationExecution
	if absent.StopRequested() || !absent.WaitDeadline().IsZero() || absent.HasPrepareIntent() || absent.RecordIntent(t.Context(), "", "", "") != nil || absent.LinkAccepted(t.Context(), "") != nil {
		t.Fatal("unbound execution changed observation")
	}
	for _, control := range []*PublicationExecution{
		{NoWait: true},
		{Detached: func() bool { return true }},
		{Deadline: time.Now().Add(-time.Second)},
	} {
		if !control.StopRequested() {
			t.Fatalf("control=%+v did not stop", control)
		}
	}
	var prepared, accepted bool
	control := &PublicationExecution{OperationID: "operation-1", Deadline: time.Now().Add(time.Hour), Prepare: func(_ context.Context, id, operation, receipt, scope string) error {
		prepared = id == "operation-1" && operation == "workbook.publish" && receipt == "receipt-1" && scope == "scope"
		return nil
	}, Accepted: func(_ context.Context, path string) error { accepted = path == "receipt.json"; return nil }}
	if control.StopRequested() || !control.HasPrepareIntent() || control.RecordIntent(t.Context(), "workbook.publish", "receipt-1", "scope") != nil || control.LinkAccepted(t.Context(), "receipt.json") != nil || !prepared || !accepted {
		t.Fatalf("prepared=%t accepted=%t", prepared, accepted)
	}
}
