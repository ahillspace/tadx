package operationrun_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestCreateReadAndRequestDoesNotPersistEnvironment(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	request := operationrun.Request{
		Args:             []string{"__publication-worker", "run-123"},
		BatchData:        json.RawMessage(`{"items":[1,2]}`),
		ConfigPath:       filepath.Join(t.TempDir(), "config.yml"),
		WorkingDirectory: t.TempDir(),
		Operation:        "workbook.publish",
		NoWait:           true,
	}
	created, err := store.Create(request)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID == "" || created.Version != 1 || created.Phase != operationrun.PhaseRequested {
		t.Fatalf("Create() record = %#v", created)
	}
	if created.Request.Operation != request.Operation {
		t.Fatalf("Create() request operation = %q, want %q", created.Request.Operation, request.Operation)
	}
	read, err := store.Read(created.ID)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if read.ID != created.ID || read.Request.NoWait != true || string(read.Request.BatchData) != string(request.BatchData) {
		t.Fatalf("Read() record = %#v", read)
	}
	data, err := os.ReadFile(filepath.Join(store.Directory, created.ID+".json"))
	if err != nil {
		t.Fatalf("read durable record: %v", err)
	}
	if string(data) == "" || string(data) == "null" {
		t.Fatal("durable record is empty")
	}
	if bytes.Contains(data, []byte(`"environment"`)) || bytes.Contains(data, []byte(`"credentials"`)) {
		t.Fatalf("durable record contains forbidden persisted input: %s", data)
	}
}

func TestConcurrentUpdatesAreAtomicAndPreserveReceipts(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	created, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	const updates = 32
	var group sync.WaitGroup
	for range updates {
		group.Go(func() {
			_, updateErr := store.Update(created.ID, func(record *operationrun.Record) error {
				var live struct {
					Count int `json:"count"`
				}
				if len(record.LiveResults) != 0 {
					if err := json.Unmarshal(record.LiveResults, &live); err != nil {
						return err
					}
				}
				live.Count++
				record.LiveResults, _ = json.Marshal(live)
				record.ReceiptPaths = []string{"receipts/" + strconv.Itoa(live.Count) + ".json"}
				return nil
			})
			if updateErr != nil {
				t.Errorf("Update() error = %v", updateErr)
			}
		})
	}
	group.Wait()

	final, err := store.Read(created.ID)
	if err != nil {
		t.Fatalf("Read() after updates error = %v", err)
	}
	var live struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(final.LiveResults, &live); err != nil {
		t.Fatalf("decode final live results: %v", err)
	}
	if live.Count != updates {
		t.Fatalf("final live result count = %d, want %d", live.Count, updates)
	}
	if final.Version != updates+1 || len(final.ReceiptPaths) != 1 {
		t.Fatalf("final record revision/receipts = %d/%v", final.Version, final.ReceiptPaths)
	}
}

func TestForegroundLeaseIsNonblockingAndIdempotent(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	created, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	first, err := store.TryAcquireForeground(created.ID)
	if err != nil {
		t.Fatalf("TryAcquireForeground() error = %v", err)
	}
	second, err := store.TryAcquireForeground(created.ID)
	if !errors.Is(err, operationrun.ErrForegroundBusy) || second != nil {
		t.Fatalf("second TryAcquireForeground() = lease %#v, error %v, want ErrForegroundBusy", second, err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("first Release() error = %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("second Release() error = %v", err)
	}
	third, err := store.TryAcquireForeground(created.ID)
	if err != nil {
		t.Fatalf("TryAcquireForeground() after release error = %v", err)
	}
	if err := third.Release(); err != nil {
		t.Fatalf("third Release() error = %v", err)
	}
}

func TestLeaseLivenessAndTerminalResult(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	created, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	lease, started, err := store.Lease(created.ID, 12345)
	if err != nil {
		t.Fatalf("Lease() error = %v", err)
	}
	defer lease.Release()
	if started.Phase != operationrun.PhaseRunning || started.WorkerPID != 12345 || started.StartedAt.IsZero() {
		t.Fatalf("Lease() acknowledgement = %#v", started)
	}
	alive, err := store.Alive(created.ID)
	if err != nil {
		t.Fatalf("Alive() error = %v", err)
	}
	if !alive {
		t.Fatal("Alive() = false while lease is held")
	}
	if _, _, err := store.Lease(created.ID, 12346); !errors.Is(err, operationrun.ErrWorkerBusy) {
		t.Fatalf("second Lease() error = %v, want ErrWorkerBusy", err)
	}

	code := 0
	finishedAt := time.Now().UTC()
	if _, err := lease.Update(func(record *operationrun.Record) error {
		record.Phase = operationrun.PhaseCompleted
		record.Detached = true
		record.Activity = "submitted all items; finalizing receipts"
		record.FinishedAt = finishedAt
		record.CompactResult = json.RawMessage(`{"status":"ok"}`)
		record.FullResult = json.RawMessage(`{"status":"ok","items":[]}`)
		record.ExitCode = &code
		return nil
	}); err != nil {
		t.Fatalf("terminal Update() error = %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	alive, err = store.Alive(created.ID)
	if err != nil {
		t.Fatalf("Alive() after Release() error = %v", err)
	}
	if alive {
		t.Fatal("Alive() = true after lease release")
	}
	final, err := store.Read(created.ID)
	if err != nil {
		t.Fatalf("Read() terminal record error = %v", err)
	}
	if final.Phase != operationrun.PhaseCompleted || !final.Detached || final.Activity == "" || final.ExitCode == nil || *final.ExitCode != 0 || final.FinishedAt.IsZero() {
		t.Fatalf("terminal record = %#v", final)
	}
	if _, _, err := store.Lease(created.ID, 12347); !errors.Is(err, operationrun.ErrTerminal) {
		t.Fatalf("terminal Lease() error = %v, want ErrTerminal", err)
	}
}

func TestLeaseDoesNotReplayInterruptedRun(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	created, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	lease, _, err := store.Lease(created.ID, 12345)
	if err != nil {
		t.Fatalf("Lease() error = %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	alive, err := store.Alive(created.ID)
	if err != nil {
		t.Fatalf("Alive() error = %v", err)
	}
	if alive {
		t.Fatal("Alive() = true after interrupted lease release")
	}
	if _, _, err := store.Lease(created.ID, 12346); !errors.Is(err, operationrun.ErrWorkerInterrupted) {
		t.Fatalf("replay Lease() error = %v, want ErrWorkerInterrupted", err)
	}
}

func TestReadRejectsRecordIDMismatch(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	first, err := store.Create(operationrun.Request{Operation: "first"})
	if err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	second, err := store.Create(operationrun.Request{Operation: "second"})
	if err != nil {
		t.Fatalf("second Create() error = %v", err)
	}
	secondData, err := os.ReadFile(filepath.Join(store.Directory, second.ID+".json"))
	if err != nil {
		t.Fatalf("read second record: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.Directory, first.ID+".json"), secondData, 0o600); err != nil {
		t.Fatalf("replace first record: %v", err)
	}
	if _, err := store.Read(first.ID); !errors.Is(err, operationrun.ErrRecordMismatch) {
		t.Fatalf("Read() mismatch error = %v, want ErrRecordMismatch", err)
	}
}

func TestInterruptedWorkerIsNotReportedAliveAfterProcessExit(t *testing.T) {
	if os.Getenv("OPERATIONRUN_LEASE_HOLDER") == "1" {
		store := operationrun.Store{Directory: os.Getenv("OPERATIONRUN_STORE")}
		lease, _, err := store.Lease(os.Getenv("OPERATIONRUN_ID"), 54321)
		if err != nil {
			t.Fatalf("holder Lease() error = %v", err)
		}
		if err := os.WriteFile(os.Getenv("OPERATIONRUN_READY"), nil, 0o600); err != nil {
			t.Fatalf("holder ready signal: %v", err)
		}
		defer lease.Release()
		for {
			time.Sleep(time.Hour)
		}
	}

	store := operationrun.Store{Directory: t.TempDir()}
	created, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	ready := filepath.Join(store.Directory, "worker-ready")
	worker := exec.Command(os.Args[0], "-test.run", "^TestInterruptedWorkerIsNotReportedAliveAfterProcessExit$")
	worker.Env = append(os.Environ(),
		"OPERATIONRUN_LEASE_HOLDER=1",
		"OPERATIONRUN_STORE="+store.Directory,
		"OPERATIONRUN_ID="+created.ID,
		"OPERATIONRUN_READY="+ready,
	)
	if err := worker.Start(); err != nil {
		t.Fatalf("start holder worker: %v", err)
	}
	waitForFile(t, ready)
	alive, err := store.Alive(created.ID)
	if err != nil {
		t.Fatalf("Alive() while holder runs error = %v", err)
	}
	if !alive {
		t.Fatal("Alive() = false while holder process runs")
	}
	if err := worker.Process.Kill(); err != nil {
		t.Fatalf("kill holder worker: %v", err)
	}
	if err := worker.Wait(); err == nil {
		t.Fatal("holder worker exited successfully after Kill()")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		alive, err = store.Alive(created.ID)
		if err != nil {
			t.Fatalf("Alive() after holder exit error = %v", err)
		}
		if !alive {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if alive {
		t.Fatal("Alive() = true after interrupted worker process exit")
	}
	record, err := store.Read(created.ID)
	if err != nil {
		t.Fatalf("Read() interrupted record error = %v", err)
	}
	if record.Phase != operationrun.PhaseRunning {
		t.Fatalf("interrupted record phase = %q, want running", record.Phase)
	}
}

func TestReadRejectsUnsafeExactID(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	for _, id := range []string{"../escape", "a/b", "", "."} {
		if _, err := store.Read(id); !errors.Is(err, operationrun.ErrInvalidID) {
			t.Fatalf("Read(%q) error = %v, want ErrInvalidID", id, err)
		}
	}
}

func TestLaunchDetachedWorkerSurvivesLauncherExit(t *testing.T) {
	if os.Getenv("OPERATIONRUN_LAUNCHER") == "1" {
		if err := operationrun.Launch(os.Args[0], []string{"-test.run", "^TestDetachedWorkerTarget$"}, ""); err != nil {
			t.Fatalf("Launch() error = %v", err)
		}
		return
	}
	if os.Getenv("OPERATIONRUN_TARGET") == "1" {
		time.Sleep(150 * time.Millisecond)
		if err := os.WriteFile(os.Getenv("OPERATIONRUN_MARKER"), []byte("done"), 0o600); err != nil {
			t.Fatalf("write detached marker: %v", err)
		}
		return
	}
	marker := filepath.Join(t.TempDir(), "detached-marker")
	launcher := exec.Command(os.Args[0], "-test.run", "^TestLaunchDetachedWorkerSurvivesLauncherExit$")
	launcher.Env = append(os.Environ(), "OPERATIONRUN_LAUNCHER=1", "OPERATIONRUN_MARKER="+marker, "OPERATIONRUN_TARGET=1")
	if err := launcher.Run(); err != nil {
		t.Fatalf("launcher process error = %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("detached worker did not survive launcher exit")
}

func TestDetachedWorkerTarget(t *testing.T) {
	if os.Getenv("OPERATIONRUN_TARGET") != "1" {
		return
	}
	time.Sleep(150 * time.Millisecond)
	if err := os.WriteFile(os.Getenv("OPERATIONRUN_MARKER"), []byte("done"), 0o600); err != nil {
		t.Fatalf("write detached marker: %v", err)
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q", path)
}
