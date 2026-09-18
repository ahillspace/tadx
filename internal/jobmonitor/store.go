// Package jobmonitor coordinates accepted jobs without submitting mutations.
package jobmonitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/lock"
	"github.com/ahillspace/tadx/internal/value"
)

const maxReceiptBytes = 64 << 10
const maxReceiptEntries = 10000

// Receipt preserves accepted identity before any wait, independently of last.
// CoordinationKey is an opaque credential/site hash, never a PAT or session.
type Receipt struct {
	ReceiptID         string          `json:"receipt_id,omitempty"`
	Version           int             `json:"version"`
	Operation         string          `json:"operation"`
	Environment       string          `json:"environment"`
	Server            string          `json:"server"`
	Site              string          `json:"site"`
	SiteID            string          `json:"site_id"`
	ConfigPath        string          `json:"config_path"`
	SourcePath        string          `json:"source_path,omitempty"`
	ProjectID         string          `json:"project_id,omitempty"`
	Name              string          `json:"name,omitempty"`
	CoordinationKey   string          `json:"coordination_key"`
	AcceptedAt        time.Time       `json:"accepted_at"`
	TrackingStartedAt time.Time       `json:"tracking_started_at,omitzero"`
	PoolAfter         time.Time       `json:"pool_after"`
	NextCheck         time.Time       `json:"next_check,omitzero"`
	Observation       value.JobStatus `json:"observation"`
	ReadFailures      int             `json:"read_failures,omitempty"`
	LastReadError     string          `json:"last_read_error,omitempty"`
	Verification      string          `json:"verification,omitempty"`
}

// Store contains no credentials and is shared by cooperating processes.
type Store struct{ Directory string }

// ReadPath reads one receipt only when it is contained by this store's
// directory. Receipt paths are local recovery inputs, never remote commands.
func (s Store) ReadPath(path string) (Receipt, error) {
	if s.Directory == "" || path == "" {
		return Receipt{}, errors.New("job receipt store is not configured")
	}
	root, err := filepath.Abs(s.Directory)
	if err != nil {
		return Receipt{}, errors.New("job receipt store path is unavailable")
	}
	requested, err := filepath.Abs(path)
	if err != nil {
		return Receipt{}, errors.New("job receipt path is unavailable")
	}
	relative, err := filepath.Rel(root, requested)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return Receipt{}, errors.New("job receipt path is outside the configured recovery store")
	}
	return readReceipt(requested)
}

// FindByJobID locates one exact receipt in the configured recovery store.
// Environment and site are optional filters used to disambiguate shared stores.
func (s Store) FindByJobID(ctx context.Context, id, environment, site string) (Receipt, string, error) {
	if s.Directory == "" || id == "" {
		return Receipt{}, "", errors.New("job receipt store is not configured")
	}
	entries, err := os.ReadDir(s.Directory)
	if err != nil {
		return Receipt{}, "", err
	}
	var found Receipt
	var foundPath string
	for index, entry := range entries {
		if index >= maxReceiptEntries {
			return Receipt{}, "", errors.New("job receipt store exceeds its recovery scan bound")
		}
		if err := ctx.Err(); err != nil {
			return Receipt{}, "", err
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		receipt, err := readReceipt(filepath.Join(s.Directory, entry.Name()))
		if err != nil {
			continue
		}
		if receipt.Observation.ID != id || (environment != "" && receipt.Environment != environment) || (site != "" && receipt.Site != site) {
			continue
		}
		if foundPath != "" {
			return Receipt{}, "", errors.New("exact job ID matches more than one durable receipt")
		}
		found, foundPath = receipt, filepath.Join(s.Directory, entry.Name())
	}
	if foundPath == "" {
		return Receipt{}, "", os.ErrNotExist
	}
	return found, foundPath, nil
}

func digest(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = fmt.Fprintf(h, "%d:%s", len(p), p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s Store) Path(r Receipt) string {
	if r.Observation.ID == "" {
		return filepath.Join(s.Directory, digest(r.Server, r.SiteID, r.ReceiptID)+".json")
	}
	return filepath.Join(s.Directory, digest(r.Server, r.SiteID, r.Observation.ID)+".json")
}

func (s Store) Save(ctx context.Context, r Receipt) (string, error) {
	if !validReceipt(r) || s.Directory == "" {
		return "", errors.New("accepted job receipt is incomplete")
	}
	if err := os.MkdirAll(s.Directory, 0700); err != nil {
		return "", err
	}
	path := s.Path(r)
	guard, err := lock.AcquireContext(ctx, path+".lock")
	if err != nil {
		return path, err
	}
	defer guard.Release()
	if previous, readErr := readReceipt(path); readErr == nil {
		if previous.Observation.Terminal() && r.Observation.Terminal() && previous.Observation.Status != r.Observation.Status {
			return path, errors.New("job observation conflicts with its saved terminal outcome")
		}
		if (previous.Observation.Terminal() && !r.Observation.Terminal()) || previous.Observation.CheckedAt.After(r.Observation.CheckedAt) {
			r.Observation = previous.Observation
			r.NextCheck, r.ReadFailures, r.LastReadError = previous.NextCheck, previous.ReadFailures, previous.LastReadError
		} else {
			if r.Observation.ResourceID == "" {
				r.Observation.ResourceID = previous.Observation.ResourceID
			}
			if r.Observation.Type == "" {
				r.Observation.Type = previous.Observation.Type
			}
		}
		if r.Verification == "" {
			r.Verification = previous.Verification
		}
		if r.AcceptedAt.IsZero() {
			r.AcceptedAt = previous.AcceptedAt
		}
		if r.TrackingStartedAt.IsZero() {
			r.TrackingStartedAt = previous.TrackingStartedAt
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return path, fmt.Errorf("read existing job receipt before saving: %w", readErr)
	}
	data, err := json.Marshal(r)
	if err != nil {
		return path, err
	}
	if len(data) > maxReceiptBytes {
		return path, errors.New("accepted job receipt exceeds its size bound")
	}
	return path, atomicWrite(path, data)
}

func (s Store) Read(r Receipt) (Receipt, error) { return readReceipt(s.Path(r)) }

func readReceipt(path string) (Receipt, error) {
	var r Receipt
	info, err := os.Lstat(path)
	if err != nil {
		return r, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxReceiptBytes {
		return r, errors.New("job receipt is not a bounded regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if len(data) > maxReceiptBytes {
		return r, errors.New("job receipt exceeds its size bound")
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return Receipt{}, err
	}
	if !validReceipt(r) {
		return Receipt{}, errors.New("unsupported or incomplete job receipt")
	}
	return r, nil
}

func validReceipt(r Receipt) bool {
	if r.Version != 1 || (r.Observation.ID == "" && r.ReceiptID == "") {
		return false
	}
	if r.Observation.ID == "" {
		return !r.AcceptedAt.IsZero()
	}
	return r.CoordinationKey != "" && (!r.AcceptedAt.IsZero() || !r.TrackingStartedAt.IsZero())
}

func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".job-receipt-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
