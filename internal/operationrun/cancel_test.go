package operationrun_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestCancellationBeforeWorkerStart(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareCancel(record.ID); err != nil {
		t.Fatal(err)
	}
	requested, err := store.RequestCancel(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if requested.RequestedAt.IsZero() || !requested.AcknowledgedAt.IsZero() || requested.Supported {
		t.Fatalf("unexpected request: %+v", requested)
	}
	repeated, err := store.RequestCancel(record.ID)
	if err != nil || repeated.RequestedAt != requested.RequestedAt {
		t.Fatalf("repeat: %+v, %v", repeated, err)
	}
	lease, _, err := store.Lease(record.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	if err := lease.BeginCommit(); !errors.Is(err, context.Canceled) {
		t.Fatalf("commit after cancellation: %v", err)
	}
	if err := lease.AcknowledgeCancel(); err != nil {
		t.Fatal(err)
	}
	observed, err := store.ReadCancel(record.ID)
	if err != nil || !observed.Supported || observed.AcknowledgedAt.IsZero() {
		t.Fatalf("observation: %+v, %v", observed, err)
	}
}

func TestCancellationControlRejectsOversize(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareCancel(record.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Directory, record.ID+".json.control.json"), []byte(strings.Repeat(" ", int(operationrun.MaxRecordBytes)+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadCancel(record.ID); !errors.Is(err, operationrun.ErrRecordTooLarge) {
		t.Fatalf("oversize control: %v", err)
	}
}

func TestCommitRequiresActiveNonterminalLease(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareCancel(record.ID); err != nil {
		t.Fatal(err)
	}
	lease, _, err := store.Lease(record.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lease.Update(func(r *operationrun.Record) error { r.Phase = operationrun.PhaseCompleted; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := lease.BeginCommit(); !errors.Is(err, operationrun.ErrTerminal) {
		t.Fatalf("terminal admission: %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
	if err := lease.BeginCommit(); !errors.Is(err, operationrun.ErrLeaseReleased) {
		t.Fatalf("released admission: %v", err)
	}
}

func TestCancellationAcrossProcesses(t *testing.T) {
	if os.Getenv("OPERATIONRUN_CANCEL_CHILD") == "1" {
		store := operationrun.Store{Directory: os.Getenv("OPERATIONRUN_CANCEL_STORE")}
		lease, _, err := store.Lease(os.Getenv("OPERATIONRUN_CANCEL_ID"), 0)
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Release()
		if err := lease.BeginCommit(); !errors.Is(err, context.Canceled) {
			t.Fatalf("commit: %v", err)
		}
		if err := lease.AcknowledgeCancel(); err != nil {
			t.Fatal(err)
		}
		return
	}
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareCancel(record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RequestCancel(record.ID); err != nil {
		t.Fatal(err)
	}
	worker := exec.Command(os.Args[0], "-test.run", "^TestCancellationAcrossProcesses$")
	worker.Env = append(os.Environ(), "OPERATIONRUN_CANCEL_CHILD=1", "OPERATIONRUN_CANCEL_STORE="+store.Directory, "OPERATIONRUN_CANCEL_ID="+record.ID)
	if output, err := worker.CombinedOutput(); err != nil {
		t.Fatalf("worker: %v\n%s", err, output)
	}
	control, err := store.ReadCancel(record.ID)
	if err != nil || !control.Supported || control.AcknowledgedAt.IsZero() {
		t.Fatalf("control: %+v, %v", control, err)
	}
}

func TestConcurrentCancellationAdmission(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareCancel(record.ID); err != nil {
		t.Fatal(err)
	}
	lease, _, err := store.Lease(record.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 20 {
		wg.Go(func() {
			<-start
			err := lease.BeginCommit()
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("admission: %v", err)
			}
		})
	}
	close(start)
	if _, err := store.RequestCancel(record.ID); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	for range 20 {
		if err := lease.BeginCommit(); !errors.Is(err, context.Canceled) {
			t.Fatalf("post-request admission: %v", err)
		}
	}
}

func TestCancellationRejectsLegacyAndTerminal(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatal(err)
	}
	lease, _, err := store.Lease(record.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	if _, err := store.RequestCancel(record.ID); !errors.Is(err, operationrun.ErrCancelUnsupported) {
		t.Fatalf("legacy cancel: %v", err)
	}
	terminal, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareCancel(terminal.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(terminal.ID, func(r *operationrun.Record) error {
		r.Phase = operationrun.PhaseCompleted
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RequestCancel(terminal.ID); !errors.Is(err, operationrun.ErrTerminal) {
		t.Fatalf("terminal cancel: %v", err)
	}
}

func TestCancellationCommitAdmission(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "pull"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareCancel(record.ID); err != nil {
		t.Fatal(err)
	}
	lease, _, err := store.Lease(record.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	if err := lease.BeginCommit(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RequestCancel(record.ID); err != nil {
		t.Fatal(err)
	}
	if err := lease.BeginCommit(); !errors.Is(err, context.Canceled) {
		t.Fatalf("commit after request: %v", err)
	}
}

func TestCancellationDoesNotChangeRecordSchema(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	before, err := store.Create(operationrun.Request{Operation: "pull"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareCancel(before.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RequestCancel(before.ID); err != nil {
		t.Fatal(err)
	}
	after, err := store.Read(before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != before.Version || after.Phase != before.Phase {
		t.Fatalf("record changed: before %+v, after %+v", before, after)
	}
	if _, err := store.RequestCancel("../other"); !errors.Is(err, operationrun.ErrInvalidID) {
		t.Fatalf("invalid ID: %v", err)
	}
}
