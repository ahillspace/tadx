package operationrun

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/fsreplace"
	"github.com/ahillspace/tadx/internal/lock"
	"github.com/ahillspace/tadx/internal/value"
)

// TDSJournal coordinates work-copy lifecycle attempts across config files and
// workspaces. A site transaction commits a pending submission and all its
// acknowledgement links together. Resource locks outlive that commit
// and stay with the action through the remote response and verification.
type TDSJournal struct{ Directory string }

func DefaultTDSJournal() (TDSJournal, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return TDSJournal{}, err
	}
	return TDSJournal{Directory: filepath.Join(d, "tadx", "tds-attempts")}, nil
}

func tdsKey(parts ...string) string {
	b, _ := json.Marshal(parts)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func sameTDSScope(a, b value.TDSScope) bool {
	return a.Server == b.Server && strings.EqualFold(a.SiteLUID, b.SiteLUID) && strings.EqualFold(a.CallerLUID, b.CallerLUID)
}

func (s TDSJournal) directory(sc value.TDSScope) (string, error) {
	if s.Directory == "" || sc.Server == "" || sc.SiteLUID == "" || sc.CallerLUID == "" {
		return "", errors.New("TDS attempt store requires an authenticated site and caller")
	}
	d := filepath.Join(s.Directory, tdsKey(sc.Server, strings.ToLower(sc.SiteLUID)))
	for _, dir := range []string{s.Directory, d} {
		if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(dir, 0700); err != nil {
				return "", err
			}
			if err := fsreplace.SyncDir(filepath.Dir(dir)); err != nil {
				return "", err
			}
		}
		info, err := os.Lstat(dir)
		if err != nil {
			return "", err
		}
		if !info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
			return "", errors.New("TDS journal requires private account-owned directories")
		}
	}
	return d, nil
}

func tdsGuards(a value.TDSAttempt) []value.TDSAttemptGuard {
	switch a.Operation {
	case "datasource.checkout":
		return []value.TDSAttemptGuard{{Kind: "checkout", ResourceLUID: a.SourceLUID, CallerLUID: a.Scope.CallerLUID}}
	case "datasource.save.overwrite":
		return []value.TDSAttemptGuard{{Kind: "workcopy", ResourceLUID: a.WorkCopyLUID}, {Kind: "overwrite", ResourceLUID: a.SourceLUID}}
	default:
		return []value.TDSAttemptGuard{{Kind: "workcopy", ResourceLUID: a.WorkCopyLUID}}
	}
}

func guardKey(g value.TDSAttemptGuard) string {
	if g.Kind == "checkout" {
		return tdsKey(g.Kind, strings.ToLower(g.CallerLUID), strings.ToLower(g.ResourceLUID))
	}
	return tdsKey(g.Kind, strings.ToLower(g.ResourceLUID))
}

// admission reads the same active guards used by Begin and by the preview. A
// match on one guard releases every guard of the acknowledged attempt.
func admission(ctx context.Context, tx *sql.Tx, draft value.TDSAttempt, acks []string) ([]string, []value.TDSAcknowledgement, error) {
	if err := validateTDSOperation(draft.Operation); err != nil {
		return nil, nil, err
	}
	var keys []string
	for _, guard := range tdsGuards(draft) {
		keys = append(keys, guardKey(guard))
	}
	first, second := keys[0], keys[len(keys)-1]
	rows, err := tx.QueryContext(ctx, `SELECT id, caller, source, work_copy, operation, status
		FROM receipts WHERE superseded='' AND (guard1 IN (?,?) OR guard2 IN (?,?)) ORDER BY id`, first, second, first, second)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var needed []string
	var matches []value.TDSAcknowledgement
	for rows.Next() {
		var a value.TDSAttempt
		if err := rows.Scan(&a.ID, &a.Scope.CallerLUID, &a.SourceLUID, &a.WorkCopyLUID, &a.Operation, &a.Status); err != nil {
			return nil, nil, err
		}
		if !strings.EqualFold(a.Scope.CallerLUID, draft.Scope.CallerLUID) {
			return nil, nil, errors.New("another caller has an unresolved submission affecting this resource")
		}
		guards := tdsGuards(a)
		matches = append(matches, value.TDSAcknowledgement{AttemptID: a.ID, Operation: a.Operation, Status: a.Status, Guards: guards})
		needed = append(needed, a.ID)
		for _, g := range guards {
			keys = append(keys, guardKey(g))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	given := slices.Clone(acks)
	sort.Strings(given)
	if !slices.Equal(given, needed) {
		if len(needed) > 0 {
			return nil, nil, fmt.Errorf("unresolved attempts %s must all be explicitly acknowledged; a new request can duplicate a prior submission", strings.Join(needed, ", "))
		}
		return nil, nil, errors.New("acknowledgements do not match any unresolved submissions")
	}
	sort.Strings(keys)
	return slices.Compact(keys), matches, nil
}

func guardFiles(d string, keys []string) (func() error, error) {
	root, err := os.OpenRoot(d)
	if err != nil {
		return nil, err
	}
	var held []*lock.Handle
	release := func() error {
		var err error
		for _, l := range slices.Backward(held) {
			err = errors.Join(err, l.Release())
		}
		return errors.Join(err, root.Close())
	}
	for _, key := range keys {
		l, err := lock.TryAcquireIn(root, key+".lock")
		if err != nil {
			return nil, errors.Join(err, release())
		}
		held = append(held, l)
	}
	return release, nil
}

func (s TDSJournal) LockCopy(ctx context.Context, sc value.TDSScope, copyID string) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d, err := s.directory(sc)
	if err != nil {
		return nil, err
	}
	return guardFiles(d, []string{tdsKey("workcopy", strings.ToLower(copyID))})
}

func (s TDSJournal) LockLifecycle(ctx context.Context, draft value.TDSAttempt, acks []string) (func() error, []value.TDSAcknowledgement, error) {
	var release func() error
	var matches []value.TDSAcknowledgement
	err := s.withLedger(ctx, draft.Scope, func(tx *sql.Tx, dir string) error {
		keys, matched, err := admission(ctx, tx, draft, acks)
		if err != nil {
			return err
		}
		release, err = guardFiles(dir, keys)
		if err == nil {
			matches = matched
		}
		return err
	})
	if err != nil && release != nil {
		return nil, nil, errors.Join(err, release())
	}
	return release, matches, err
}

func (s TDSJournal) Begin(ctx context.Context, draft value.TDSAttempt, acks []string) (value.TDSAttempt, error) {
	err := s.withLedger(ctx, draft.Scope, func(tx *sql.Tx, _ string) error {
		if _, _, err := admission(ctx, tx, draft, acks); err != nil {
			return err
		}
		id, err := newID()
		if err != nil {
			return err
		}
		draft.ID, draft.Status, draft.UpdatedAt = id, "pending", time.Now().UTC()
		if err := storeTDSAttempt(ctx, tx, draft, admitSubmission); err != nil {
			return err
		}
		for _, prior := range acks {
			if _, err := tx.ExecContext(ctx, `UPDATE receipts SET superseded=?, updated=? WHERE id=?`, id, draft.UpdatedAt.UnixNano(), prior); err != nil {
				return err
			}
		}
		return nil
	})
	return draft, err
}

func (s TDSJournal) Record(ctx context.Context, a value.TDSAttempt) error {
	return s.withLedger(ctx, a.Scope, func(tx *sql.Tx, _ string) error {
		var superseded, status string
		err := tx.QueryRowContext(ctx, `SELECT superseded,status FROM receipts WHERE id=? AND caller=? AND operation=? AND attempt IS NOT NULL`, a.ID, a.Scope.CallerLUID, a.Operation).Scan(&superseded, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if superseded != "" {
			return errors.New("attempt has been superseded")
		}
		if status == "acknowledged" && a.Status != "acknowledged" {
			return errors.New("cannot replace an acknowledged submission with an uncertain outcome")
		}
		a.UpdatedAt = time.Now().UTC()
		if err := storeTDSAttempt(ctx, tx, a, retainOutcome); err != nil {
			return err
		}
		// If nothing was dispatched the original guards become active together.
		if a.Status == "not_attempted" {
			_, err = tx.ExecContext(ctx, `UPDATE receipts SET superseded='',updated=? WHERE superseded=?`, a.UpdatedAt.UnixNano(), a.ID)
		}
		return err
	})
}

func (s TDSJournal) Attempts(ctx context.Context, sc value.TDSScope, id, sourceID, copyID string) ([]value.TDSAttempt, bool, error) {
	matches := []value.TDSAttempt{}
	more := false
	err := s.withLedger(ctx, sc, func(tx *sql.Tx, _ string) error {
		query := `SELECT COALESCE(attempt,origin),attempt IS NULL,superseded,updated FROM receipts WHERE caller=?`
		args := []any{sc.CallerLUID}
		if id != "" {
			query += " AND id=? AND attempt IS NOT NULL"
			args = append(args, id)
		}
		if sourceID != "" {
			query += " AND source=?"
			args = append(args, sourceID)
		}
		if copyID != "" {
			query += " AND work_copy=?"
			args = append(args, copyID)
		}
		rows, err := tx.QueryContext(ctx, query+" ORDER BY updated DESC, rowid DESC LIMIT 101", args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		var size int
		for rows.Next() {
			if len(matches) == 100 {
				more = true
				break
			}
			var data []byte
			var provenanceOnly bool
			var superseded string
			var updated int64
			if err := rows.Scan(&data, &provenanceOnly, &superseded, &updated); err != nil {
				return err
			}
			if len(matches) > 0 && size+len(data) > ledgerLimit {
				more = true
				break
			}
			size += len(data)
			var a value.TDSAttempt
			if err := json.Unmarshal(data, &a); err != nil {
				return err
			}
			if !sameTDSScope(a.Scope, sc) {
				return errors.New("invalid TDS receipt scope")
			}
			a.SupersededBy, a.UpdatedAt = superseded, time.Unix(0, updated).UTC()
			if provenanceOnly {
				a.ProvenanceOnly = true
				a.ID = ""
			}
			matches = append(matches, a)
		}
		return rows.Err()
	})
	if err == nil && id != "" && len(matches) == 0 {
		err = ErrNotFound
	}
	return matches, more, err
}

func (s TDSJournal) Origin(ctx context.Context, sc value.TDSScope, copyID string) (value.TDSAttempt, error) {
	var origin value.TDSAttempt
	err := s.withLedger(ctx, sc, func(tx *sql.Tx, _ string) error {
		var data []byte
		err := tx.QueryRowContext(ctx, `SELECT origin FROM receipts WHERE caller=? AND work_copy=? AND origin IS NOT NULL ORDER BY updated DESC, rowid DESC LIMIT 1`, sc.CallerLUID, copyID).Scan(&data)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &origin); err != nil {
			return err
		}
		if !sameTDSScope(origin.Scope, sc) || !strings.EqualFold(origin.WorkCopyLUID, copyID) {
			return errors.New("invalid TDS checkout provenance scope")
		}
		var conflict bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM receipts WHERE caller=? AND work_copy=? AND source<>? AND origin IS NOT NULL)`, sc.CallerLUID, copyID, origin.SourceLUID).Scan(&conflict)
		if err == nil && conflict {
			return errors.New("checkout records disagree about the source")
		}
		return err
	})
	if err != nil {
		return value.TDSAttempt{}, err
	}
	return origin, nil
}
