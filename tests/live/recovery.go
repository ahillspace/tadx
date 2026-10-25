package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ahillspace/tadx/tests/live/harness"
)

// guardLocalWorkers runs under the target selector lock before authentication.
// Local worker observations must precede every PAT session handoff, including a
// handoff after an interrupted prior process. Remote jobs remain a later guard.
func guardLocalWorkers(ctx context.Context, p harness.Profile, binary, state string) error {
	journals, err := localJournalPaths(ctx, state)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(state, "manifest.json")
	info, err := os.Lstat(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		for _, path := range journals {
			data, readErr := os.ReadFile(path)
			if readErr != nil || len(data) > 8<<20 {
				return harness.Inconclusive("local journal cannot be validated without its target manifest; authentication is deferred")
			}
			var header struct {
				Target string `json:"target_fingerprint"`
			}
			if json.Unmarshal(data, &header) != nil || header.Target == "" {
				return harness.Inconclusive("local journal target header is invalid; authentication is deferred")
			}
			journal, loadErr := harness.LoadJournal(path, header.Target)
			if loadErr != nil || !journalFullyCleaned(journal) {
				return harness.Inconclusive("unfinished or invalid local journals lack a target manifest; preserve them before authenticating")
			}
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect target manifest before authentication: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return harness.Blocked("target manifest must be a regular local file before authentication")
	}
	manifest, err := loadTargetManifest(state, p)
	if err != nil {
		return err
	}
	for _, path := range journals {
		if err = ctx.Err(); err != nil {
			return err
		}
		journal, err := harness.LoadJournal(path, manifest.TargetFingerprint)
		if err != nil {
			return harness.Inconclusive("a local cleanup journal is invalid or belongs to another target; authentication is deferred")
		}
		if journalFullyCleaned(journal) {
			continue
		}
		resumed, err := harness.ResumeCase(ctx, p, binary, filepath.Dir(path), manifest.TargetFingerprint, nil, []string{"job.inspect"})
		if err != nil {
			return fmt.Errorf("restore local worker observation context before authentication: %w", err)
		}
		if err = resumed.GuardWorkerStopped(ctx); err != nil {
			return err
		}
	}
	return nil
}

// localJournalPaths never follows a substituted state tree. A missing runs
// directory is a fresh local state; unreadable or special entries fail closed.
func localJournalPaths(ctx context.Context, state string) ([]string, error) {
	if info, err := os.Lstat(state); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, harness.Blocked("live state must be a local directory before authentication")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	root := filepath.Join(state, "runs")
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, harness.Blocked("live runs must be a local directory before authentication")
	}
	var paths []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return harness.Blocked("live recovery refuses symbolic links before authentication")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return harness.Blocked("live recovery refuses special filesystem entries")
		}
		if entry.Name() == "journal.json" {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

// Completion authorizes only a local authentication handoff, never deletion or
// adoption of journal objects against a different target.
func journalFullyCleaned(journal *harness.Journal) bool {
	if journal == nil || journal.TransferPending {
		return false
	}
	for _, entry := range journal.Entries {
		if !entry.Done {
			return false
		}
	}
	return true
}
