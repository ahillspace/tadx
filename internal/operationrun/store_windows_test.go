package operationrun_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/ahillspace/tadx/internal/operationrun"
)

// Windows cannot replace a file while another handle to it is open, and the
// standard library opens files without FILE_SHARE_DELETE. These tests hold the
// durable record open the way a concurrent lookup or file scanner would.

func TestUpdateReplacesRecordAfterTransientHandleCloses(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	created, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	holder, err := os.Open(filepath.Join(store.Directory, created.ID+".json"))
	if err != nil {
		t.Fatalf("open durable record: %v", err)
	}
	defer holder.Close()

	type outcome struct {
		record operationrun.Record
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		record, err := store.Update(created.ID, func(record *operationrun.Record) error {
			record.Activity = "replaced after the handle closed"
			return nil
		})
		done <- outcome{record: record, err: err}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for len(temporaryRecords(t, store.Directory, created.ID)) == 0 {
		select {
		case got := <-done:
			t.Fatalf("Update() returned before its replacement was staged: record %#v, error %v", got.record, got.err)
		default:
		}
		if !time.Now().Before(deadline) {
			t.Fatal("Update() did not stage a replacement record")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case got := <-done:
		t.Fatalf("Update() returned while the record was held open: record %#v, error %v", got.record, got.err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := holder.Close(); err != nil {
		t.Fatalf("close durable record: %v", err)
	}

	var got outcome
	select {
	case got = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Update() did not finish after the record handle closed")
	}
	if got.err != nil {
		t.Fatalf("Update() error = %v", got.err)
	}
	if got.record.Version != 2 || got.record.Activity != "replaced after the handle closed" {
		t.Fatalf("Update() record = %#v", got.record)
	}
	final, err := store.Read(created.ID)
	if err != nil {
		t.Fatalf("Read() after update error = %v", err)
	}
	if final.Version != 2 || final.Activity != got.record.Activity {
		t.Fatalf("durable record revision/activity = %d/%q, want 2/%q", final.Version, final.Activity, got.record.Activity)
	}
	if staged := temporaryRecords(t, store.Directory, created.ID); len(staged) != 0 {
		t.Fatalf("temporary records remain after update: %v", staged)
	}
}

func TestUpdateReportsReplaceFailureWhileRecordStaysOpen(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	created, err := store.Create(operationrun.Request{Operation: "publish"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	holder, err := os.Open(filepath.Join(store.Directory, created.ID+".json"))
	if err != nil {
		t.Fatalf("open durable record: %v", err)
	}
	defer holder.Close()

	done := make(chan error, 1)
	go func() {
		_, err := store.Update(created.ID, func(record *operationrun.Record) error {
			record.Activity = "must not persist"
			return nil
		})
		done <- err
	}()
	var updateErr error
	select {
	case updateErr = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Update() kept retrying while the record stayed open")
	}
	if !errors.Is(updateErr, windows.ERROR_ACCESS_DENIED) && !errors.Is(updateErr, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("Update() error = %v, want the replace sharing failure", updateErr)
	}
	if err := holder.Close(); err != nil {
		t.Fatalf("close durable record: %v", err)
	}

	final, err := store.Read(created.ID)
	if err != nil {
		t.Fatalf("Read() after failed update error = %v", err)
	}
	if final.Version != 1 || final.Activity != "" {
		t.Fatalf("durable record revision/activity after failed update = %d/%q, want 1/empty", final.Version, final.Activity)
	}
	if staged := temporaryRecords(t, store.Directory, created.ID); len(staged) != 0 {
		t.Fatalf("temporary records remain after failed update: %v", staged)
	}
}

func temporaryRecords(t *testing.T, directory, id string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("list operation run store: %v", err)
	}
	var staged []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "."+id+".json.tmp-") {
			staged = append(staged, entry.Name())
		}
	}
	return staged
}
