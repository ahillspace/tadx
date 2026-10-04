package operationrun_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestCoordinatorLaunchFailureRetainsIdentityWithoutRetry(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	calls := 0
	cause := errors.New("launch acknowledgement lost")
	coordinator := operationrun.Coordinator{Store: store, Launch: func(context.Context, string, string) error { calls++; return cause }}
	out, err := coordinator.Begin(t.Context(), operationrun.Request{Operation: "workbook.publish"})
	failure, ok := errors.AsType[*errs.Error](err)
	if !ok || failure.ID != "operation.worker.start" || !errors.Is(err, cause) || calls != 1 || !out.Present || out.Record.ID == "" {
		t.Fatalf("out=%+v err=%v calls=%d", out, err, calls)
	}
	saved, err := store.Read(out.Record.ID)
	if err != nil || saved.Phase != operationrun.PhaseRequested {
		t.Fatalf("record=%+v err=%v", saved, err)
	}
}

func TestCoordinatorCompletedFailureRetainsRenderedResult(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	coordinator := operationrun.Coordinator{Store: store, Launch: func(_ context.Context, _, id string) error {
		worker, err := operationrun.OpenWorker(store, id, func(string) bool { return true })
		if err != nil {
			return err
		}
		defer worker.Close()
		return worker.Finish(json.RawMessage("{}"), json.RawMessage("{}"), 1, false, nil)
	}}
	out, err := coordinator.Begin(t.Context(), operationrun.Request{Operation: "workbook.publish"})
	failure, ok := errors.AsType[*errs.Error](err)
	if !ok || failure.ID != "operation.worker.failed" || !out.Present || !out.Rendered || out.Record.Phase != operationrun.PhaseFailed {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestWorkerSnapshotFailurePreservesReceiptAndPartialResult(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "datasource.publish"})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := operationrun.OpenWorker(store, record.ID, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	if err := worker.PrepareIntent(record.ID, record.Operation, "receipt", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	if err := worker.AcceptReceipt("receipt.json"); err != nil {
		t.Fatal(err)
	}
	if err := worker.SaveProgress(json.RawMessage("{\"accepted\":true}"), "Accepted"); err != nil {
		t.Fatal(err)
	}
	captureErr := errors.New("expanded result exceeds bound")
	if err := worker.Finish(nil, nil, 0, false, captureErr); !errors.Is(err, captureErr) {
		t.Fatalf("err=%v", err)
	}
	saved, err := store.Read(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Phase != operationrun.PhaseRemotePending || saved.ExitCode == nil || *saved.ExitCode != 1 || len(saved.ReceiptPaths) != 1 || len(saved.ReceiptIntents) != 1 || string(saved.LiveResults) != "{\"accepted\":true}" {
		t.Fatalf("saved=%+v", saved)
	}
}

func TestWorkerRejectsWrongIntentBeforePersistence(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "flow.publish"})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := operationrun.OpenWorker(store, record.ID, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	if err := worker.PrepareIntent("different-operation", record.Operation, "receipt", "scope"); err == nil {
		t.Fatal("accepted mismatched operation")
	}
	saved, err := store.Read(record.ID)
	if err != nil || len(saved.ReceiptIntents) != 0 {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}
}
