package live

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/tests/live/harness"
)

type targetState struct {
	Path             string   `json:"path"`
	EvidenceRequired bool     `json:"evidence_required,omitzero"`
	Journals         []string `json:"journals,omitempty"`
}

type targetStates struct {
	Version int           `json:"version"`
	Target  string        `json:"target"`
	States  []targetState `json:"states"`
}

func loadTargetStates(path, target string) (targetStates, error) {
	var registry targetStates
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return targetStates{Version: 1, Target: target}, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1<<20 {
		return registry, harness.Blocked("target recovery registry is unavailable or not a bounded regular local file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return registry, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&registry); err != nil {
		return registry, harness.Blocked("target recovery registry is invalid; preserve its recovery evidence")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) || registry.Version != 1 || registry.Target != target || registry.States == nil {
		return registry, harness.Blocked("target recovery registry version or target is invalid")
	}
	seen := map[string]bool{}
	for _, entry := range registry.States {
		if !filepath.IsAbs(entry.Path) || filepath.Clean(entry.Path) != entry.Path || seen[entry.Path] {
			return registry, harness.Blocked("target recovery registry contains invalid or duplicate state paths")
		}
		seen[entry.Path] = true
		for _, journal := range entry.Journals {
			if filepath.IsAbs(journal) || filepath.ToSlash(filepath.Clean(journal)) != journal || strings.HasPrefix(journal, "../") || filepath.Base(journal) != "journal.json" {
				return registry, harness.Blocked("target recovery registry contains an unsafe journal path")
			}
		}
	}
	return registry, nil
}

// guardTargetWorkers runs under the shared target lock. Register before
// authentication so an interrupted phase in another checkout remains visible.
func guardTargetWorkers(ctx context.Context, p harness.Profile, binary, state string) error {
	target := harness.Fingerprint(p.ServerURL, p.SiteContentURL, "")
	path, err := harness.TargetStateRegistryPath(target)
	if err != nil {
		return err
	}
	registry, err := loadTargetStates(path, target)
	if err != nil {
		return err
	}
	current, err := filepath.Abs(state)
	if err != nil {
		return err
	}
	found := false
	for _, entry := range registry.States {
		if entry.Path == current {
			found = true
		}
	}
	if !found {
		_, manifestErr := os.Lstat(filepath.Join(current, "manifest.json"))
		journals, err := localJournalPaths(ctx, current)
		if err != nil {
			return err
		}
		entry := targetState{Path: current, EvidenceRequired: manifestErr == nil}
		for _, journal := range journals {
			relative, err := filepath.Rel(current, journal)
			if err != nil {
				return err
			}
			entry.Journals = append(entry.Journals, filepath.ToSlash(relative))
		}
		registry.States = append(registry.States, entry)
	}
	if err := harness.WriteJSON(path, registry); err != nil {
		return err
	}
	retained := make([]targetState, 0, len(registry.States))
	for _, entry := range registry.States {
		journals, err := localJournalPaths(ctx, entry.Path)
		if err != nil {
			return err
		}
		_, stateErr := os.Lstat(entry.Path)
		_, manifestErr := os.Lstat(filepath.Join(entry.Path, "manifest.json"))
		if entry.EvidenceRequired && (stateErr != nil || (errors.Is(manifestErr, os.ErrNotExist) && len(journals) == 0)) {
			return harness.Inconclusive("registered target state lost its recovery evidence; authentication is deferred")
		}
		for _, journal := range entry.Journals {
			if !slices.Contains(journals, filepath.Join(entry.Path, filepath.FromSlash(journal))) {
				return harness.Inconclusive("a registered ownership journal is missing; authentication is deferred")
			}
		}
		if err := guardLocalWorkers(ctx, p, binary, entry.Path); err != nil {
			return err
		}
		if manifestErr == nil {
			entry.EvidenceRequired = true
		}
		// No manifest and only fully cleaned journals prove that this state can
		// no longer own a worker. Fresh current state remains registered.
		if entry.Path == current || !errors.Is(manifestErr, os.ErrNotExist) {
			retained = append(retained, entry)
		}
	}
	registry.States = retained
	return harness.WriteJSON(path, registry)
}

// requireTargetRecoveryEvidence precedes the first possible remote write.
// A later missing state cannot then be mistaken for an unused registration.
func requireTargetRecoveryEvidence(p harness.Profile, state, journalPath string) error {
	target := harness.Fingerprint(p.ServerURL, p.SiteContentURL, "")
	path, err := harness.TargetStateRegistryPath(target)
	if err != nil {
		return err
	}
	registry, err := loadTargetStates(path, target)
	if err != nil {
		return err
	}
	current, err := filepath.Abs(state)
	if err != nil {
		return err
	}
	for index := range registry.States {
		if registry.States[index].Path == current {
			registry.States[index].EvidenceRequired = true
			relative, err := filepath.Rel(current, journalPath)
			if err != nil || filepath.IsAbs(relative) || strings.HasPrefix(filepath.ToSlash(relative), "../") || filepath.Base(relative) != "journal.json" {
				return harness.Blocked("target recovery journal must remain inside its registered state")
			}
			if _, err := os.Lstat(journalPath); err != nil {
				return err
			}
			journal := filepath.ToSlash(relative)
			if !slices.Contains(registry.States[index].Journals, journal) {
				registry.States[index].Journals = append(registry.States[index].Journals, journal)
			}
			return harness.WriteJSON(path, registry)
		}
	}
	return harness.Blocked("target state was not registered before preparing its recovery evidence")
}

// finishTargetState is called only after exact-target setup cleanup has
// verified every dependent journal and removed the final setup manifest.
func finishTargetState(p harness.Profile, state string) error {
	target := harness.Fingerprint(p.ServerURL, p.SiteContentURL, "")
	path, err := harness.TargetStateRegistryPath(target)
	if err != nil {
		return err
	}
	registry, err := loadTargetStates(path, target)
	if err != nil {
		return err
	}
	current, err := filepath.Abs(state)
	if err != nil {
		return err
	}
	retained := make([]targetState, 0, len(registry.States))
	for _, entry := range registry.States {
		if entry.Path != current {
			retained = append(retained, entry)
		}
	}
	registry.States = retained
	return harness.WriteJSON(path, registry)
}

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
