package operationrun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
// workspaces. A site's small ledger commits a pending submission and all its
// acknowledgement links with one rename. Resource locks outlive that commit
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

const ledgerLimit = 16 << 20

// withLedger keeps I/O and admission atomic without holding a site-wide lock
// during a Tableau request. Corrupt or oversized state fails closed.
func (s TDSJournal) withLedger(ctx context.Context, sc value.TDSScope, write bool, fn func(*[]value.TDSAttempt) error) error {
	d, err := s.directory(sc)
	if err != nil {
		return err
	}
	l, err := lock.AcquireContext(ctx, filepath.Join(d, "ledger.lock"))
	if err != nil {
		return err
	}
	defer l.Release()
	path := filepath.Join(d, "attempts.json")
	var records []value.TDSAttempt
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
			return errors.New("invalid TDS journal file permissions or type")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.Open(path)
	if err == nil {
		data, rerr := io.ReadAll(io.LimitReader(file, ledgerLimit+1))
		cerr := file.Close()
		if err := errors.Join(rerr, cerr); err != nil {
			return err
		}
		if len(data) > ledgerLimit {
			return ErrRecordTooLarge
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&records); err != nil {
			return err
		}
		if records == nil || dec.Decode(new(any)) != io.EOF {
			return errors.New("invalid TDS attempt ledger")
		}
		for _, a := range records {
			if a.Scope.Server != sc.Server || !strings.EqualFold(a.Scope.SiteLUID, sc.SiteLUID) || a.Scope.CallerLUID == "" || a.ID == "" || a.UpdatedAt.IsZero() {
				return errors.New("TDS attempt identity is incomplete")
			}
			switch a.Operation {
			case "datasource.checkout", "datasource.save.overwrite", "datasource.save.as-new":
			default:
				return errors.New("invalid TDS attempt operation")
			}
			switch a.Status {
			case "acknowledged", "pending", "unknown", "not_applied", "not_attempted":
			default:
				return errors.New("invalid TDS submit status")
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	beforePrune := len(records)
	records = slices.DeleteFunc(records, func(a value.TDSAttempt) bool {
		return a.UpdatedAt.Before(cutoff) && !blocking(a) && !(a.Operation == "datasource.checkout" && a.WorkCopyLUID != "" && a.Status == "acknowledged")
	})
	if err := fn(&records); err != nil {
		return err
	}
	if !write && beforePrune == len(records) {
		return nil
	}
	data, err := json.Marshal(records)
	if err != nil {
		return err
	}
	if len(data) > ledgerLimit {
		return ErrRecordTooLarge
	}
	return installRecord(path, data)
}

func blocking(a value.TDSAttempt) bool {
	return (a.Status == "pending" || a.Status == "unknown") && a.SupersededBy == ""
}
func tdsGuards(a value.TDSAttempt) []string {
	switch a.Operation {
	case "datasource.checkout":
		return []string{tdsKey("checkout", strings.ToLower(a.Scope.CallerLUID), strings.ToLower(a.SourceLUID))}
	case "datasource.save.overwrite":
		return []string{tdsKey("workcopy", strings.ToLower(a.WorkCopyLUID)), tdsKey("overwrite", strings.ToLower(a.SourceLUID))}
	default:
		return []string{tdsKey("workcopy", strings.ToLower(a.WorkCopyLUID))}
	}
}

// admission selects the full blocker set. Another caller's blocker remains
// effective without exposing its attempt ID or scope in the diagnostic.
func admission(records []value.TDSAttempt, draft value.TDSAttempt, acks []string) ([]string, error) {
	desired := tdsGuards(draft)
	guards := slices.Clone(desired)
	var needed []string
	for _, a := range records {
		if !blocking(a) {
			continue
		}
		keys := tdsGuards(a)
		if !slices.ContainsFunc(keys, func(k string) bool { return slices.Contains(desired, k) }) {
			continue
		}
		if !strings.EqualFold(a.Scope.CallerLUID, draft.Scope.CallerLUID) {
			return nil, errors.New("another caller has an unresolved submission affecting this resource")
		}
		needed = append(needed, a.ID)
		guards = append(guards, keys...)
	}
	sort.Strings(needed)
	given := slices.Clone(acks)
	sort.Strings(given)
	if !slices.Equal(given, needed) {
		if len(needed) > 0 {
			return nil, fmt.Errorf("unresolved attempts %s must all be explicitly acknowledged; a new request can duplicate a prior submission", strings.Join(needed, ", "))
		}
		return nil, errors.New("acknowledgements do not match any unresolved submissions")
	}
	sort.Strings(guards)
	return slices.Compact(guards), nil
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

func (s TDSJournal) LockLifecycle(ctx context.Context, draft value.TDSAttempt, acks []string) (func() error, error) {
	d, err := s.directory(draft.Scope)
	if err != nil {
		return nil, err
	}
	var release func() error
	err = s.withLedger(ctx, draft.Scope, false, func(r *[]value.TDSAttempt) error {
		keys, err := admission(*r, draft, acks)
		if err != nil {
			return err
		}
		release, err = guardFiles(d, keys)
		return err
	})
	if err != nil && release != nil {
		return nil, errors.Join(err, release())
	}
	return release, err
}

func (s TDSJournal) Begin(ctx context.Context, draft value.TDSAttempt, acks []string) (value.TDSAttempt, error) {
	err := s.withLedger(ctx, draft.Scope, true, func(records *[]value.TDSAttempt) error {
		if _, err := admission(*records, draft, acks); err != nil {
			return err
		}
		id, err := newID()
		if err != nil {
			return err
		}
		draft.ID = id
		draft.Status = "pending"
		draft.UpdatedAt = time.Now().UTC()
		for i := range *records {
			a := &(*records)[i]
			if slices.Contains(acks, a.ID) {
				a.SupersededBy = id
				a.UpdatedAt = draft.UpdatedAt
			}
		}
		*records = append(*records, draft)
		return nil
	})
	return draft, err
}

func (s TDSJournal) Record(ctx context.Context, a value.TDSAttempt) error {
	return s.withLedger(ctx, a.Scope, true, func(records *[]value.TDSAttempt) error {
		for i, old := range *records {
			if old.ID == a.ID && sameTDSScope(old.Scope, a.Scope) && old.Operation == a.Operation {
				if old.SupersededBy != "" {
					return errors.New("attempt has been superseded")
				}
				a.UpdatedAt = time.Now().UTC()
				// A stopped local request did not spend the user's acknowledgement on a
				// fresh submit. Restore the original blockers in the same ledger commit.
				if a.Status == "not_attempted" {
					for n := range *records {
						prior := &(*records)[n]
						if prior.SupersededBy == a.ID {
							prior.SupersededBy = ""
							prior.UpdatedAt = a.UpdatedAt
						}
					}
				}
				(*records)[i] = a
				return nil
			}
		}
		return ErrNotFound
	})
}

func (s TDSJournal) Attempts(ctx context.Context, sc value.TDSScope, id, sourceID, copyID string) ([]value.TDSAttempt, bool, error) {
	matches := []value.TDSAttempt{}
	err := s.withLedger(ctx, sc, false, func(records *[]value.TDSAttempt) error {
		for _, a := range slices.Backward(*records) {
			if sameTDSScope(a.Scope, sc) && (id == "" || a.ID == id) && (sourceID == "" || strings.EqualFold(a.SourceLUID, sourceID)) && (copyID == "" || strings.EqualFold(a.WorkCopyLUID, copyID)) {
				matches = append(matches, a)
			}
		}
		if id != "" && len(matches) == 0 {
			return ErrNotFound
		}
		return nil
	})
	slices.SortStableFunc(matches, func(a, b value.TDSAttempt) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	if len(matches) > 100 {
		return matches[:100], true, err
	}
	return matches, false, err
}

func (s TDSJournal) Origin(ctx context.Context, sc value.TDSScope, copyID string) (value.TDSAttempt, error) {
	var result value.TDSAttempt
	err := s.withLedger(ctx, sc, false, func(records *[]value.TDSAttempt) error {
		for _, a := range *records {
			if !sameTDSScope(a.Scope, sc) || a.Operation != "datasource.checkout" || a.Status != "acknowledged" || !strings.EqualFold(a.WorkCopyLUID, copyID) {
				continue
			}
			if result.ID != "" && !strings.EqualFold(result.SourceLUID, a.SourceLUID) {
				return errors.New("checkout records disagree about the source")
			}
			result = a
		}
		return nil
	})
	return result, err
}
