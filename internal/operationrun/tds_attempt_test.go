package operationrun_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/value"
)

func TestTDSAdmissionSurvivesAnInterruptedOwner(t *testing.T) {
	ctx := context.Background()
	store := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "tds-attempts")}
	scope := value.TDSScope{Server: "https://example.test", SiteLUID: "site", CallerLUID: "caller"}
	draft := value.TDSAttempt{Scope: scope, Operation: "datasource.checkout", SourceLUID: "source"}
	unlock, _, err := store.LockLifecycle(ctx, draft, nil)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := store.Begin(ctx, draft, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LockLifecycle(ctx, draft, []string{pending.ID}); err == nil {
		t.Fatal("admitted while first process still owns checkout")
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LockLifecycle(ctx, draft, nil); err == nil {
		t.Fatal("lost pending blocker after interruption")
	}
	release, _, err := store.LockLifecycle(ctx, draft, []string{pending.ID})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	next, err := store.Begin(ctx, draft, []string{pending.ID})
	if err != nil {
		t.Fatal(err)
	}
	records, more, err := store.Attempts(ctx, scope, "", "source", "")
	if err != nil || more || len(records) != 2 || records[0].ID != next.ID || records[1].SupersededBy != next.ID || records[1].Status != "pending" {
		t.Fatalf("records=%+v more=%v err=%v", records, more, err)
	}
}

func TestTDSAcknowledgesAllOverlappingUnknownsOrNone(t *testing.T) {
	ctx := context.Background()
	s := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "tds-attempts")}
	sc := value.TDSScope{Server: "https://tableau.example", SiteLUID: "site", CallerLUID: "one"}
	a := value.TDSAttempt{Scope: sc, Operation: "datasource.save.as-new", WorkCopyLUID: "copy-1"}
	b := value.TDSAttempt{Scope: sc, Operation: "datasource.save.overwrite", WorkCopyLUID: "copy-2", SourceLUID: "original"}
	a, err := s.Begin(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err = s.Begin(ctx, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	draft := value.TDSAttempt{Scope: sc, Operation: b.Operation, WorkCopyLUID: a.WorkCopyLUID, SourceLUID: b.SourceLUID}
	for _, ack := range [][]string{nil, {a.ID}, {b.ID}, {b.ID, b.ID}, {a.ID, b.ID, "extra"}} {
		if _, _, err := s.LockLifecycle(ctx, draft, ack); err == nil {
			t.Fatalf("accepted acknowledgements %v", ack)
		}
	}
	other := draft
	other.Scope.CallerLUID = "other"
	if _, matches, err := s.LockLifecycle(ctx, other, []string{a.ID, b.ID}); err == nil || len(matches) != 0 || strings.Contains(err.Error(), a.ID) || strings.Contains(err.Error(), b.ID) {
		t.Fatalf("foreign blocker exposed or ignored: %v", err)
	}
	if _, _, err := s.Attempts(ctx, other.Scope, a.ID, "", ""); err == nil {
		t.Fatal("foreign lookup succeeded")
	}
	release, _, err := s.LockLifecycle(ctx, draft, []string{a.ID, b.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LockCopy(ctx, sc, b.WorkCopyLUID); err == nil {
		t.Fatal("acknowledged guard missing from held union")
	}
	// A crash before Begin must leave both earlier blockers intact.
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.LockLifecycle(ctx, draft, nil); err == nil {
		t.Fatal("lost previous blockers before new pending commit")
	}
	release, _, err = s.LockLifecycle(ctx, draft, []string{a.ID, b.ID})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	pending, err := s.Begin(ctx, draft, []string{b.ID, a.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{a.ID, b.ID} {
		r, _, err := s.Attempts(ctx, sc, id, "", "")
		if err != nil || len(r) != 1 || r[0].SupersededBy != pending.ID || r[0].Status != "pending" {
			t.Fatalf("lost acknowledgement %+v %v", r, err)
		}
	}
}

func TestTDSJournalIgnoresProfileAndUUIDCase(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG config root fixture")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, err := operationrun.DefaultTDSJournal()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sc := value.TDSScope{Server: "https://example.test", SiteLUID: "DEAD-BEEF", CallerLUID: "AaaA"}
	req := value.TDSAttempt{Scope: sc, SourceLUID: "EEFF", Operation: "datasource.checkout"}
	a, err := s.Begin(ctx, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := operationrun.DefaultTDSJournal()
	if err != nil || s != other {
		t.Fatal(other, err)
	}
	req.SourceLUID = "eeff"
	req.Scope.CallerLUID = "aaaa"
	req.Scope.SiteLUID = "dead-beef"
	if _, _, err := other.LockLifecycle(ctx, req, nil); err == nil || !strings.Contains(err.Error(), a.ID) {
		t.Fatal("UUID case variation escaped the original blocker", err)
	}
	rows, _, err := other.Attempts(ctx, req.Scope, "", "eeff", "")
	if err != nil || len(rows) != 1 || rows[0].ID != a.ID {
		t.Fatal(rows, err)
	}
}

func TestTDSJournalRejectsLinkedStateAndWorldReadableReceipts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions and symbolic links fixture")
	}
	ctx := context.Background()
	sc := value.TDSScope{Server: "https://tableau.example", SiteLUID: "site", CallerLUID: "caller"}
	store := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "state")}
	a, err := store.Begin(ctx, value.TDSAttempt{Scope: sc, Operation: "datasource.checkout", SourceLUID: "source"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(store.Directory, "*", "attempts.json"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	path := files[0]
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private receipt %v %v", info, err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Attempts(ctx, sc, a.ID, "", ""); err == nil {
		t.Fatal("accepted exposed ledger")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), path+"-journal"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Attempts(ctx, sc, a.ID, "", ""); err == nil {
		t.Fatal("accepted linked database journal")
	}
	if err := os.Remove(path + "-journal"); err != nil {
		t.Fatal(err)
	}
	alt := filepath.Join(t.TempDir(), "state")
	if err := os.Symlink(store.Directory, alt); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (operationrun.TDSJournal{Directory: alt}).Attempts(ctx, sc, a.ID, "", ""); err == nil {
		t.Fatal("accepted linked journal")
	}
}
