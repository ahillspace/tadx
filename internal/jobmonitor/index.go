package jobmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/ahillspace/tadx/internal/lock"
)

const maxActiveJobs = 1000

func (s Store) indexPath(key string) string {
	return filepath.Join(s.Directory, "active-"+digest(key)+".json")
}

// Register saves acceptance before adding the job to the shared monitoring pool.
// An index failure still leaves the accepted receipt available for recovery.
func (s Store) Register(ctx context.Context, r Receipt) (string, error) {
	if r.Observation.ID == "" {
		return "", errors.New("only an accepted remote job can enter monitoring")
	}
	path, err := s.Save(ctx, r)
	if err != nil {
		return path, err
	}
	indexPath := s.indexPath(r.CoordinationKey)
	guard, err := lock.AcquireContext(ctx, indexPath+".lock")
	if err != nil {
		return path, err
	}
	defer guard.Release()
	paths, err := readIndex(indexPath)
	if err != nil {
		return path, err
	}
	name := filepath.Base(path)
	if slices.Contains(paths, name) {
		return path, nil
	}
	// Completed entries leave the active index; their receipts remain intact.
	active := paths[:0]
	for _, p := range paths {
		previous, readErr := readReceipt(filepath.Join(s.Directory, p))
		if readErr != nil || !previous.Observation.Terminal() {
			active = append(active, p)
		}
	}
	if len(active) >= maxActiveJobs {
		return path, errors.New("active job index is full; accepted receipt remains saved")
	}
	data, err := json.Marshal(append(active, name))
	if err != nil {
		return path, err
	}
	return path, atomicWrite(indexPath, data)
}

func readIndex(path string) ([]string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxActiveJobs*80 {
		return nil, errors.New("active job index is not a bounded regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, err
	}
	if len(paths) > maxActiveJobs {
		return nil, errors.New("active job index exceeds its bound")
	}
	for _, p := range paths {
		if len(p) != 69 || filepath.Base(p) != p || filepath.Ext(p) != ".json" {
			return nil, errors.New("invalid active job receipt path")
		}
	}
	return paths, nil
}
