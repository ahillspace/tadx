package operationrun

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/value"
)

func writeLegacyTDSJournal(t *testing.T, store TDSJournal, sc value.TDSScope, records []value.TDSAttempt) string {
	t.Helper()
	dir, err := store.directory(sc)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > ledgerLimit {
		t.Fatalf("fixture exceeds the original format limit: %d", len(data))
	}
	path := filepath.Join(dir, "attempts.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTDSNearFullLedgerRetainsConcurrentSubmitIdentities(t *testing.T) {
	store := TDSJournal{Directory: filepath.Join(t.TempDir(), "journal")}
	sc := value.TDSScope{Server: "https://example.test", SiteLUID: "site", CallerLUID: "caller"}
	// All rows are still within retention. Pruning cannot free admission capacity.
	var legacy []value.TDSAttempt
	for i := 0; i < 1024; i++ {
		legacy = append(legacy, value.TDSAttempt{ID: fmt.Sprintf("run-%032x", i), Scope: sc, Operation: "datasource.checkout", Status: "acknowledged", SourceLUID: "source", WorkCopyLUID: fmt.Sprintf("legacy-copy-%d", i), IntentDigest: "old-intent", Checkout: &value.TDSCheckoutContext{SourceName: strings.Repeat("x", 15000)}, UpdatedAt: time.Now().UTC()})
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacy[0].Checkout.SourceName += strings.Repeat("x", ledgerLimit-len(data)-1536)
	writeLegacyTDSJournal(t, store, sc, legacy)
	pending := []value.TDSAttempt{
		{Scope: sc, Operation: "datasource.checkout", SourceLUID: "new-source"},
		{Scope: sc, Operation: "datasource.save.as-new", WorkCopyLUID: "new-copy"},
		{Scope: sc, Operation: "datasource.save.overwrite", SourceLUID: "another-source", WorkCopyLUID: "other-copy"},
	}
	var wg sync.WaitGroup
	for i := range pending {
		wg.Go(func() {
			attempt, err := store.Begin(t.Context(), pending[i], nil)
			if err != nil {
				t.Errorf("admit independent attempt: %v", err)
				return
			}
			pending[i] = attempt
		})
	}
	wg.Wait()
	if t.Failed() {
		return
	}
	// Go's HTML escaping expands this maximum-size printable Tableau request
	// identifier sixfold in JSON. No other pending attempt can spend its room.
	requestID := strings.Repeat("<", 16<<10)
	for _, a := range pending {
		wg.Go(func() {
			a.Status, a.RequestID = "acknowledged", requestID
			switch a.Operation {
			case "datasource.checkout":
				a.WorkCopyLUID = "allocated-copy"
			case "datasource.save.as-new":
				a.PublishedLUID = "new-publication"
			case "datasource.save.overwrite":
				a.PublishedLUID = a.SourceLUID
			}
			if err := store.Record(t.Context(), a); err != nil {
				t.Errorf("admitted attempt lost acknowledgement: %v", err)
				return
			}
			reopened := TDSJournal{Directory: store.Directory}
			rows, _, err := reopened.Attempts(t.Context(), sc, a.ID, "", "")
			if err != nil || len(rows) != 1 || rows[0].WorkCopyLUID != a.WorkCopyLUID || rows[0].PublishedLUID != a.PublishedLUID || rows[0].RequestID != requestID {
				t.Errorf("reopening lost the copy, publication, or request identity: %v", err)
			}
		})
	}
	wg.Wait()
}

func TestTDSExpiredCheckoutHistoryRetainsScopedProvenance(t *testing.T) {
	store := TDSJournal{Directory: filepath.Join(t.TempDir(), "journal")}
	sc := value.TDSScope{Server: "https://example.test", SiteLUID: "site", CallerLUID: "caller"}
	old := time.Now().Add(-31 * 24 * time.Hour).UTC()
	c := value.TDSAttempt{ID: "old-checkout", Scope: sc, Operation: "datasource.checkout", Status: "acknowledged", SourceLUID: "source", WorkCopyLUID: "copy", SourceRevision: "v1", Checkout: &value.TDSCheckoutContext{SourceName: "Original name", AcknowledgedAt: old.Format(time.RFC3339Nano)}, RequestID: "old-provider-request", IntentDigest: "old-request-digest", UpdatedAt: old}
	uncertain := value.TDSAttempt{ID: "uncertain", Scope: sc, Operation: "datasource.save.overwrite", Status: "pending", WorkCopyLUID: "copy", SourceLUID: "source", UpdatedAt: old}
	closed := value.TDSAttempt{ID: "closed", Scope: sc, Operation: "datasource.save.as-new", Status: "unknown", SupersededBy: "new", WorkCopyLUID: "another-copy", UpdatedAt: old}
	path := writeLegacyTDSJournal(t, store, sc, []value.TDSAttempt{c, uncertain, closed})
	for _, id := range []string{c.ID, closed.ID} {
		if _, _, err := store.Attempts(t.Context(), sc, id, "", ""); !errors.Is(err, ErrNotFound) {
			t.Errorf("expired attempt %q is still recoverable: %v", id, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || bytes.Contains(data, []byte(c.RequestID)) || bytes.Contains(data, []byte(c.IntentDigest)) {
		t.Fatalf("expired checkout history remains in private database pages: %v", err)
	}
	wantOrigin := func(sc value.TDSScope, want bool) {
		t.Helper()
		origin, err := store.Origin(t.Context(), sc, "COPY")
		if err != nil || (want && (origin.SourceLUID != c.SourceLUID || origin.SourceRevision != "v1" || origin.Checkout == nil || *origin.Checkout != *c.Checkout)) || (!want && origin.SourceLUID != "") {
			t.Fatalf("origin %+v: %v", origin, err)
		}
	}
	wantOrigin(sc, true)
	foreign := sc
	foreign.CallerLUID = "elsewhere"
	wantOrigin(foreign, false)
	rows, more, err := store.Attempts(t.Context(), sc, "", "SOURCE", "cOpY")
	if err != nil || more || len(rows) != 2 {
		t.Fatalf("expected uncertain attempt and active checkout: %+v %v", rows, err)
	}
	for _, r := range rows {
		if r.ID == uncertain.ID {
			continue
		}
		if !r.ProvenanceOnly || r.ID != "" || r.WorkCopyLUID != "copy" || r.Checkout == nil || r.RequestID != "" || r.IntentDigest != "" {
			t.Fatalf("provenance leaked expired attempt history: %+v", r)
		}
	}
	if rows, _, err := store.Attempts(t.Context(), foreign, "", "", "copy"); err != nil || len(rows) != 0 {
		t.Fatalf("foreign history: %+v %v", rows, err)
	}
	if _, err := store.Begin(t.Context(), uncertain, nil); err == nil {
		t.Fatal("old pending guard expired")
	}
}

func TestTDSJournalUpgradeResumesWithTheLegacyOwnersFinalReceipt(t *testing.T) {
	j := TDSJournal{Directory: filepath.Join(t.TempDir(), "journal")}
	sc := value.TDSScope{Server: "https://example.test", SiteLUID: "site", CallerLUID: "user"}
	legacy := value.TDSAttempt{ID: "legacy", Scope: sc, Operation: "datasource.checkout", Status: "pending", SourceLUID: "source", UpdatedAt: time.Now().UTC()}
	path := writeLegacyTDSJournal(t, j, sc, []value.TDSAttempt{legacy})
	dir := filepath.Dir(path)
	// Use the original lock hash and filename, independently of the current
	// guard projection, to model an older binary still awaiting Tableau.
	release, err := guardFiles(dir, []string{tdsKey("checkout", "user", "source")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := j.Attempts(t.Context(), sc, "", "", ""); err == nil {
		t.Fatal("upgraded while old binary still had an in-flight submission")
	}
	legacy.Status, legacy.WorkCopyLUID = "acknowledged", "confirmed-copy"
	writeLegacyTDSJournal(t, j, sc, []value.TDSAttempt{legacy})
	if err := release(); err != nil {
		t.Fatal(err)
	}
	// Leave a partial staging file as if the upgrader had stopped before
	// its rename. The original JSON still owns the latest acknowledgement.
	stage := filepath.Join(dir, ".attempts-migration.sqlite")
	for _, file := range []string{stage, stage + "-journal"} {
		if err := os.WriteFile(file, []byte("incomplete stage"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	rows, _, err := j.Attempts(t.Context(), sc, legacy.ID, "", "")
	if err != nil || len(rows) != 1 || rows[0].WorkCopyLUID != legacy.WorkCopyLUID {
		t.Fatalf("lost legacy acknowledgement: %+v %v", rows, err)
	}
	for _, file := range []string{stage, stage + "-journal"} {
		if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("uncommitted migration remains: %v", err)
		}
	}
	installed, err := os.ReadFile(path)
	if err != nil || json.Valid(installed) {
		t.Fatalf("older clients would still accept the obsolete ledger: %v", err)
	}
}

func TestTDSJournalFailedMigrationPreservesScopedLegacyState(t *testing.T) {
	j := TDSJournal{Directory: filepath.Join(t.TempDir(), "journal")}
	sc := value.TDSScope{Server: "https://example.test", SiteLUID: "site", CallerLUID: "user"}
	a := value.TDSAttempt{ID: "existing", Scope: sc, Operation: "datasource.checkout", Status: "pending", SourceLUID: "source", UpdatedAt: time.Now().UTC()}
	foreign := a
	foreign.ID = "foreign"
	foreign.Scope.SiteLUID = "other-site"
	path := writeLegacyTDSJournal(t, j, sc, []value.TDSAttempt{a, foreign})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Begin(t.Context(), a, []string{a.ID}); err == nil {
		t.Fatal("discarded a foreign record during migration")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("modified legacy state after failed import: %v", err)
	}
}

func TestTDSFailedAcknowledgementLinkRollsBackNewAttempt(t *testing.T) {
	j := TDSJournal{Directory: filepath.Join(t.TempDir(), "journal")}
	sc := value.TDSScope{Server: "https://example.test", SiteLUID: "site", CallerLUID: "caller"}
	a, err := j.Begin(t.Context(), value.TDSAttempt{Scope: sc, Operation: "datasource.save.as-new", WorkCopyLUID: "copy-a"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := j.Begin(t.Context(), value.TDSAttempt{Scope: sc, Operation: "datasource.save.overwrite", SourceLUID: "source", WorkCopyLUID: "copy-b"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := j.directory(sc)
	if err != nil {
		t.Fatal(err)
	}
	db, err := openTDSDatabase(filepath.Join(dir, "attempts.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Abort the second link after the transaction inserted the follow-up
	// pending record and linked its first acknowledgement.
	_, err = db.Exec(`CREATE TRIGGER interrupt_link BEFORE UPDATE OF superseded ON receipts
		WHEN OLD.work_copy='copy-b' BEGIN SELECT RAISE(ABORT,'interrupted second link'); END`)
	if err := errors.Join(err, db.Close()); err != nil {
		t.Fatal(err)
	}
	proposed := value.TDSAttempt{Scope: sc, Operation: "datasource.save.overwrite", WorkCopyLUID: a.WorkCopyLUID, SourceLUID: b.SourceLUID}
	if _, err := j.Begin(t.Context(), proposed, []string{a.ID, b.ID}); err == nil {
		t.Fatal("admitted a linked submission despite an interrupted commit")
	}
	rows, _, err := j.Attempts(t.Context(), sc, "", "", "")
	if err != nil || len(rows) != 2 {
		t.Fatalf("partial commit: %+v %v", rows, err)
	}
	for _, row := range rows {
		if row.SupersededBy != "" || row.Status != "pending" {
			t.Fatalf("lost prior blocking attempt: %+v", row)
		}
	}
}
