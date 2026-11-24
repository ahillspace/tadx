package operationrun

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/fsreplace"
	"github.com/ahillspace/tadx/internal/lock"
	"github.com/ahillspace/tadx/internal/value"
	_ "modernc.org/sqlite"
)

// The original per-site JSON ledger and an individual new submission plan
// share this bound. A provider receipt can grow beyond its admitted plan.
const ledgerLimit = 16 << 20

// Attempt history and checkout provenance have independent lifetimes. They
// share scoped identity and can be committed in a single row update. Guard
// keys remain present on superseded uncertainties so a pre-dispatch abort can
// restore the original blockers without reconstructing their outcomes.
const tdsJournalSchema = `
CREATE TABLE journal(version INTEGER NOT NULL, server TEXT NOT NULL, site TEXT NOT NULL);
CREATE TABLE receipts(
  id TEXT PRIMARY KEY, caller TEXT NOT NULL COLLATE NOCASE,
  source TEXT NOT NULL COLLATE NOCASE, work_copy TEXT NOT NULL COLLATE NOCASE,
  operation TEXT NOT NULL, status TEXT NOT NULL, updated INTEGER NOT NULL,
  superseded TEXT NOT NULL, guard1 TEXT NOT NULL, guard2 TEXT NOT NULL,
  attempt BLOB, origin BLOB, CHECK(attempt IS NOT NULL OR origin IS NOT NULL)
);
CREATE INDEX recent ON receipts(caller,updated);
CREATE INDEX copies ON receipts(caller,work_copy) WHERE origin IS NOT NULL;
CREATE INDEX first_guard ON receipts(guard1) WHERE superseded='';
CREATE INDEX second_guard ON receipts(guard2) WHERE superseded='';
CREATE INDEX expires ON receipts(updated) WHERE attempt IS NOT NULL AND (guard1='' OR superseded<>'');
`

// The site lock covers format migration and transactions only. Tableau calls
// keep their resource locks and never hold the database or the site lock.
func (s TDSJournal) withLedger(ctx context.Context, sc value.TDSScope, fn func(*sql.Tx, string) error) error {
	dir, err := s.directory(sc)
	if err != nil {
		return err
	}
	lockPath := filepath.Join(dir, "ledger.lock")
	if _, err := privateTDSFile(lockPath); err != nil {
		return err
	}
	l, err := lock.AcquireContext(ctx, lockPath)
	if err != nil {
		return err
	}
	defer l.Release()
	db, err := openTDSJournal(ctx, dir, sc)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	var server, site string
	if err := tx.QueryRowContext(ctx, `SELECT version,server,site FROM journal`).Scan(&version, &server, &site); err != nil {
		return err
	}
	if version != 1 || server != sc.Server || !strings.EqualFold(site, sc.SiteLUID) {
		return errors.New("unsupported TDS journal version or incorrect site identity")
	}
	cutoff := time.Now().Add(-30 * 24 * time.Hour).UnixNano()
	const expired = `attempt IS NOT NULL AND (guard1='' OR superseded<>'') AND updated<?`
	if _, err := tx.ExecContext(ctx, `DELETE FROM receipts WHERE origin IS NULL AND `+expired, cutoff); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE receipts SET attempt=NULL WHERE origin IS NOT NULL AND `+expired, cutoff); err != nil {
		return err
	}
	if err := fn(tx, dir); err != nil {
		return err
	}
	return tx.Commit()
}

func privateTDSFile(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return false, errors.New("invalid TDS journal file permissions or type")
	}
	return true, nil
}

func openTDSDatabase(path string) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	location := filepath.ToSlash(absolute)
	if !strings.HasPrefix(location, "/") {
		location = "/" + location
	}
	dsn := url.URL{Scheme: "file", Path: location, RawQuery: "mode=rw&_pragma=synchronous(EXTRA)&_pragma=secure_delete(ON)&_pragma=trusted_schema(OFF)"}
	return sql.Open("sqlite", dsn.String())
}

func openTDSJournal(ctx context.Context, dir string, sc value.TDSScope) (*sql.DB, error) {
	// Keep the historical filename so replacing it is the sole format switch.
	// Older binaries fail closed on the SQLite header instead of starting a
	// second ledger that cannot see the new blockers.
	path := filepath.Join(dir, "attempts.json")
	exists, err := privateTDSFile(path)
	if err != nil {
		return nil, err
	}
	hasSidecar := false
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		sidecar, err := privateTDSFile(path + suffix)
		if err != nil {
			return nil, err
		}
		hasSidecar = hasSidecar || sidecar
	}
	var legacy []byte
	if exists {
		isDatabase, records, err := readTDSFormat(path)
		if err != nil {
			return nil, err
		}
		if isDatabase {
			return openTDSDatabase(path)
		}
		legacy = records
	}
	if hasSidecar {
		return nil, errors.New("a TDS journal sidecar exists without its SQLite database")
	}
	if err := migrateTDSJournal(ctx, path, sc, legacy); err != nil {
		return nil, err
	}
	return openTDSDatabase(path)
}

func readTDSFormat(path string) (bool, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, nil, err
	}
	defer f.Close()
	var header [16]byte
	if _, err = f.ReadAt(header[:], 0); err != nil && !errors.Is(err, io.EOF) {
		return false, nil, err
	}
	if string(header[:]) == "SQLite format 3\x00" {
		return true, nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(f, ledgerLimit+1))
	if err != nil {
		return false, nil, err
	}
	if len(data) > ledgerLimit {
		return false, nil, ErrRecordTooLarge
	}
	if len(data) == 0 {
		return false, nil, errors.New("empty TDS attempt ledger")
	}
	return false, data, nil
}

// An old binary that has already dispatched must finish writing its JSON
// outcome before migration. Under ledger.lock no old process can enter Begin.
// Try existing resource locks only; thousands of idle receipts need no locks.
func checkLegacyTDSOwner(root *os.Root, checked map[string]bool, a value.TDSAttempt) error {
	for _, guard := range tdsGuards(a) {
		name := guardKey(guard) + ".lock"
		if checked[name] {
			continue
		}
		checked[name] = true
		if _, err := root.Lstat(name); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		held, err := lock.TryAcquireIn(root, name)
		if errors.Is(err, lock.ErrLocked) {
			return errors.New("an earlier TADX process still owns a TDS submission; finish that operation before upgrading its journal")
		}
		if err != nil {
			return err
		}
		if err := held.Release(); err != nil {
			return err
		}
	}
	return nil
}

func migrateTDSJournal(ctx context.Context, path string, sc value.TDSScope, legacy []byte) error {
	dir := filepath.Dir(path)
	// A stopped migration leaves the original authoritative. Rebuild its
	// private staging database at the next open, including any hot journal.
	stage := filepath.Join(dir, ".attempts-migration.sqlite")
	for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
		if err := os.Remove(stage + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	f, err := os.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer os.Remove(stage)
	if err := f.Close(); err != nil {
		return err
	}
	db, err := openTDSDatabase(stage)
	if err != nil {
		return err
	}
	err = initializeTDSDatabase(ctx, db, dir, sc, legacy)
	if err := errors.Join(err, db.Close()); err != nil {
		return err
	}
	return fsreplace.Replace(stage, path)
}

func initializeTDSDatabase(ctx context.Context, db *sql.DB, dir string, sc value.TDSScope, legacy []byte) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, tdsJournalSchema); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO journal VALUES (1,?,?)`, sc.Server, sc.SiteLUID); err != nil {
		return err
	}
	if legacy != nil {
		if err := importLegacyTDSAttempts(ctx, tx, dir, sc, legacy); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Decode and validate one legacy record at a time. The bounded file may
// contain many short rows, including malformed ones.
func importLegacyTDSAttempts(ctx context.Context, tx *sql.Tx, dir string, sc value.TDSScope, legacy []byte) error {
	dec := json.NewDecoder(bytes.NewReader(legacy))
	dec.DisallowUnknownFields()
	if token, err := dec.Token(); err != nil || token != json.Delim('[') {
		return errors.New("invalid TDS attempt ledger")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	checked := map[string]bool{}
	for dec.More() {
		var a value.TDSAttempt
		if err := dec.Decode(&a); err != nil {
			return err
		}
		if a.Scope.Server != sc.Server || !strings.EqualFold(a.Scope.SiteLUID, sc.SiteLUID) {
			return errors.New("invalid TDS attempt site")
		}
		if err := checkLegacyTDSOwner(root, checked, a); err != nil {
			return err
		}
		if err := storeTDSAttempt(ctx, tx, a, importHistory); err != nil {
			return err
		}
	}
	if end, err := dec.Token(); err != nil || end != json.Delim(']') {
		return errors.New("invalid TDS attempt ledger")
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("trailing TDS attempt ledger data")
	}
	return nil
}

func validateTDSOperation(operation string) error {
	switch operation {
	case "datasource.checkout", "datasource.save.overwrite", "datasource.save.as-new":
		return nil
	default:
		return errors.New("invalid TDS attempt operation")
	}
}

type tdsReceiptWrite int

const (
	importHistory tdsReceiptWrite = iota
	admitSubmission
	retainOutcome
)

func storeTDSAttempt(ctx context.Context, tx *sql.Tx, a value.TDSAttempt, write tdsReceiptWrite) error {
	if a.ID == "" || a.Scope.CallerLUID == "" || a.UpdatedAt.IsZero() || a.ProvenanceOnly {
		return errors.New("incomplete TDS attempt")
	}
	if err := validateTDSOperation(a.Operation); err != nil {
		return err
	}
	var first, second string
	switch a.Status {
	case "pending", "unknown":
		guards := tdsGuards(a)
		first = guardKey(guards[0])
		if len(guards) > 1 {
			second = guardKey(guards[1])
		}
	case "acknowledged", "not_applied", "not_attempted":
	default:
		return errors.New("invalid TDS submit status")
	}
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if write == admitSubmission && len(data) > ledgerLimit {
		return ErrRecordTooLarge
	}
	var origin []byte
	if a.Operation == "datasource.checkout" && a.Status == "acknowledged" && a.WorkCopyLUID != "" {
		origin, err = json.Marshal(value.TDSAttempt{
			ID: a.ID, Scope: a.Scope, Operation: a.Operation, Status: a.Status,
			SourceLUID: a.SourceLUID, WorkCopyLUID: a.WorkCopyLUID, SourceRevision: a.SourceRevision,
			Checkout: a.Checkout, UpdatedAt: a.UpdatedAt,
		})
		if err != nil {
			return err
		}
	}
	if write == retainOutcome {
		_, err = tx.ExecContext(ctx, `UPDATE receipts SET source=?,work_copy=?,status=?,updated=?,guard1=?,guard2=?,attempt=?,origin=? WHERE id=?`,
			a.SourceLUID, a.WorkCopyLUID, a.Status, a.UpdatedAt.UnixNano(), first, second, data, origin, a.ID)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO receipts(id,caller,source,work_copy,operation,status,updated,superseded,guard1,guard2,attempt,origin) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			a.ID, a.Scope.CallerLUID, a.SourceLUID, a.WorkCopyLUID, a.Operation, a.Status, a.UpdatedAt.UnixNano(), a.SupersededBy, first, second, data, origin)
	}
	if err != nil {
		return fmt.Errorf("store TDS receipt: %w", err)
	}
	return nil
}
