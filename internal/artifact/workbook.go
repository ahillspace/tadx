// Package artifact owns deterministic local artifact persistence and dirty guards.
package artifact

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/lock"
)

// WorkbookMetadata is the frozen workbook provenance contract.
type WorkbookMetadata struct {
	Kind                     string `json:"kind"`
	Name                     string `json:"name"`
	TableauID                string `json:"tableau_id"`
	SourceServerOrigin       string `json:"source_server_origin"`
	SourceSiteLUID           string `json:"source_site_luid"`
	SourceEnvironment        string `json:"source_environment,omitempty"`
	SourceSite               string `json:"source_site"`
	SourceProjectName        string `json:"source_project_name,omitempty"`
	SourceProjectID          string `json:"source_project_id,omitempty"`
	PulledAt                 string `json:"pulled_at"`
	CanonicalPayload         string `json:"canonical_payload"`
	LocalBaselineFingerprint string `json:"local_baseline_fingerprint"`
	// Portability reports whether the workbook is self-contained, bound to its
	// source site by published-datasource references, or not conclusively detected.
	// Empty remains readable for artifacts created before explicit unknown state.
	Portability string `json:"portability,omitempty"`
	// PublishedDatasources are the direct published-datasource references detected
	// in the workbook. Presence implies Portability "source-site-bound".
	PublishedDatasources []PublishedDatasourceRef `json:"published_datasources,omitempty"`
	// DependenciesAcquired reports that referenced published datasources were
	// downloaded as sibling artifacts (via --include-pds).
	DependenciesAcquired bool `json:"dependencies_acquired,omitempty"`
	sourceSitePresent    bool
}

// PublishedDatasourceRef is one direct published-datasource reference recorded
// on a workbook artifact. The LUID is authoritative; Name and SourceSite are
// labels. LocalArtifactPath is set only when the dependency was acquired as a
// sibling datasource artifact.
type PublishedDatasourceRef struct {
	LUID              string `json:"luid"`
	Name              string `json:"name,omitempty"`
	SourceSite        string `json:"source_site,omitempty"`
	LocalArtifactPath string `json:"local_artifact_path,omitempty"`
}

// Portability values recorded on a workbook artifact.
const (
	PortabilityPortable        = "portable"
	PortabilitySourceSiteBound = "source-site-bound"
	PortabilityUnknown         = "unknown"
)

// WorkbookPull is one complete local workbook replacement request.
type WorkbookPull struct {
	Workspace string
	Filename  string
	Content   []byte
	Metadata  WorkbookMetadata
	Overwrite bool
}

// WorkbookPullResult reports the materialized artifact.
type WorkbookPullResult struct {
	ArtifactPath        string
	CanonicalPath       string
	BaselineFingerprint string
	Warnings            []string
}

// WorkbookArtifact is the current canonical payload used for publish.
type WorkbookArtifact struct {
	Path                     string
	PayloadPath              string
	Filename                 string
	Size                     int64
	Name                     string
	TableauID                string
	Fingerprint              string
	SourceEnvironment        string
	SourceSite               string
	SourceProjectName        string
	SourceProjectID          string
	Portability              string
	PublishedDatasourceCount int
}

// WorkbookManager owns workbook artifact storage.
type WorkbookManager struct{ now func() time.Time }

// NewWorkbookManager creates an artifact manager.
func NewWorkbookManager(now func() time.Time) *WorkbookManager {
	if now == nil {
		now = time.Now
	}
	return &WorkbookManager{now: now}
}

// Pull creates or safely replaces one identity-matched artifact.
func (m *WorkbookManager) Pull(ctx context.Context, input WorkbookPull) (WorkbookPullResult, error) {
	if err := ctx.Err(); err != nil {
		return WorkbookPullResult{}, err
	}
	if input.Workspace == "" || input.Metadata.TableauID == "" || input.Metadata.Name == "" {
		return WorkbookPullResult{}, errors.New("workbook artifact requires workspace, Tableau ID, and name")
	}
	serverOrigin, err := NormalizeServerOrigin(input.Metadata.SourceServerOrigin)
	if err != nil {
		return WorkbookPullResult{}, err
	}
	input.Metadata.SourceServerOrigin = serverOrigin
	input.Metadata.SourceSiteLUID = strings.TrimSpace(input.Metadata.SourceSiteLUID)
	if input.Metadata.SourceSiteLUID == "" {
		return WorkbookPullResult{}, errors.New("workbook artifact metadata requires source_site_luid")
	}
	filename := filepath.Base(input.Filename)
	nativeExtension := filepath.Ext(filename)
	extension := strings.ToLower(nativeExtension)
	if extension != ".twb" && extension != ".twbx" {
		return WorkbookPullResult{}, fmt.Errorf("unsupported workbook artifact filename %q", input.Filename)
	}
	filename = portableComponent(strings.TrimSuffix(filename, nativeExtension), "workbook", maxPortableComponentBytes-len(nativeExtension)) + nativeExtension
	workspace, err := filepath.Abs(input.Workspace)
	if err != nil {
		return WorkbookPullResult{}, err
	}
	if _, err := os.Stat(filepath.Join(workspace, "tadx.yaml")); err != nil {
		return WorkbookPullResult{}, fmt.Errorf("workspace %q does not contain tadx.yaml", workspace)
	}
	// Serialize the whole identity-match-and-replace critical section against
	// other tadx processes on this workspace: concurrent pulls would otherwise
	// race on the shared backup/stage window and corrupt or orphan state.
	handle, err := lock.Acquire(filepath.Join(workspace, ".tadx.lock"))
	if err != nil {
		return WorkbookPullResult{}, fmt.Errorf("lock workspace %q: %w", workspace, err)
	}
	defer func() { _ = handle.Release() }()
	root, err := ensureWorkbookRoot(workspace)
	if err != nil {
		return WorkbookPullResult{}, err
	}
	var warnings []string
	recoveryWarnings, err := recoverWorkbookRoot(root, defaultDirectoryOperations())
	if err != nil {
		return WorkbookPullResult{}, err
	}
	warnings = append(warnings, recoveryWarnings...)
	target, existing, err := findBySourceIdentity(root, input.Metadata.SourceServerOrigin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID)
	if err != nil {
		return WorkbookPullResult{}, err
	}
	if target == "" {
		target = filepath.Join(root, identityComponent(input.Metadata.Name, input.Metadata.SourceServerOrigin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID))
		if info, statErr := os.Lstat(target); statErr == nil {
			if err := validateContainedPath(root, target); err != nil {
				return WorkbookPullResult{}, err
			}
			if !info.IsDir() {
				return WorkbookPullResult{}, fmt.Errorf("artifact path %q is not a managed workbook directory", target)
			}
			metadata, readErr := readMetadata(target)
			if readErr != nil {
				return WorkbookPullResult{}, fmt.Errorf("artifact path %q is not a managed workbook artifact: %w", target, readErr)
			}
			if !sameSourceIdentity(metadata, input.Metadata.SourceServerOrigin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID) {
				return WorkbookPullResult{}, fmt.Errorf("artifact path %q belongs to a different Tableau source identity", target)
			}
			if err := validateManagedWorkbook(target, metadata); err != nil {
				return WorkbookPullResult{}, fmt.Errorf("artifact path %q is not a managed workbook artifact: %w", target, err)
			}
			existing = &metadata
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return WorkbookPullResult{}, statErr
		}
	}
	if existing != nil {
		canonical, err := canonicalWorkbookPath(target, existing.CanonicalPayload)
		if err != nil {
			return WorkbookPullResult{}, err
		}
		currentFingerprint, err := fingerprintFile(ctx, canonical)
		if err != nil {
			return WorkbookPullResult{}, fmt.Errorf("fingerprint existing canonical workbook: %w", err)
		}
		dirty := currentFingerprint != existing.LocalBaselineFingerprint
		if dirty && !input.Overwrite {
			return WorkbookPullResult{}, fmt.Errorf("workbook artifact %q is dirty; use --overwrite to replace local edits", target)
		}
		if dirty {
			warnings = append(warnings, "dirty workbook artifact replaced because --overwrite was provided")
		} else {
			warnings = append(warnings, "clean workbook artifact refreshed from Tableau")
		}
	}
	baseline := fingerprint(input.Content)
	metadata := input.Metadata
	metadata.Kind = "workbook"
	metadata.PulledAt = m.now().UTC().Format(time.RFC3339Nano)
	metadata.CanonicalPayload = filename
	metadata.LocalBaselineFingerprint = baseline
	metadata.sourceSitePresent = true
	if err := validateWorkbookMetadata(metadata); err != nil {
		return WorkbookPullResult{}, err
	}
	metadataBytes, err := encodeWorkbookMetadata(metadata)
	if err != nil {
		return WorkbookPullResult{}, err
	}
	view := workbookView(metadata)
	operations := defaultDirectoryOperations()
	staging, err := os.MkdirTemp(root, ".tadx-workbook-stage-")
	if err != nil {
		return WorkbookPullResult{}, fmt.Errorf("create artifact staging directory: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := writeStagedArtifact(staging, map[string][]byte{filename: input.Content, "metadata.json": metadataBytes, "view.md": []byte(view)}, operations); err != nil {
		return WorkbookPullResult{}, err
	}
	replacementWarnings, err := replaceDirectoryWithOperations(staging, target, operations)
	if err != nil {
		return WorkbookPullResult{}, err
	}
	warnings = append(warnings, replacementWarnings...)
	return WorkbookPullResult{ArtifactPath: target, CanonicalPath: filepath.Join(target, filename), BaselineFingerprint: baseline, Warnings: warnings}, nil
}

func encodeWorkbookMetadata(metadata WorkbookMetadata) ([]byte, error) {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode workbook metadata: %w", err)
	}
	data = append(data, '\n')
	if len(data) > maxWorkbookMetadataBytes {
		return nil, fmt.Errorf("workbook metadata exceeds %d-byte limit", maxWorkbookMetadataBytes)
	}
	return data, nil
}

// Read validates the current native workbook and returns its publishable file contract.
func (m *WorkbookManager) Read(ctx context.Context, path string) (WorkbookArtifact, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return WorkbookArtifact{}, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return WorkbookArtifact{}, fmt.Errorf("inspect workbook artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return WorkbookArtifact{}, fmt.Errorf("workbook artifact selector %q must not be a symbolic link", path)
	}
	directory := absolute
	if !info.IsDir() {
		directory = filepath.Dir(absolute)
	}
	metadata, err := readMetadata(directory)
	if err != nil {
		return WorkbookArtifact{}, err
	}
	if err := validateWorkbookMetadata(metadata); err != nil {
		return WorkbookArtifact{}, err
	}
	if err := validateWorkbookView(directory); err != nil {
		return WorkbookArtifact{}, err
	}
	canonical, err := canonicalWorkbookPath(directory, metadata.CanonicalPayload)
	if err != nil {
		return WorkbookArtifact{}, err
	}
	canonicalInfo, err := os.Stat(canonical)
	if err != nil {
		return WorkbookArtifact{}, fmt.Errorf("inspect canonical workbook: %w", err)
	}
	if !info.IsDir() && !os.SameFile(info, canonicalInfo) {
		return WorkbookArtifact{}, fmt.Errorf("workbook artifact file %q is not the canonical payload %q", path, metadata.CanonicalPayload)
	}
	currentFingerprint, err := fingerprintFile(ctx, canonical)
	if err != nil {
		return WorkbookArtifact{}, fmt.Errorf("fingerprint canonical workbook: %w", err)
	}
	return WorkbookArtifact{Path: directory, PayloadPath: canonical, Filename: metadata.CanonicalPayload, Size: canonicalInfo.Size(), Name: metadata.Name, TableauID: metadata.TableauID, Fingerprint: currentFingerprint, SourceEnvironment: metadata.SourceEnvironment, SourceSite: metadata.SourceSite, SourceProjectName: metadata.SourceProjectName, SourceProjectID: metadata.SourceProjectID, Portability: metadata.Portability, PublishedDatasourceCount: len(metadata.PublishedDatasources)}, nil
}

// ReadMetadata parses and validates one workbook artifact's recorded provenance
// without touching the canonical payload or recomputing its fingerprint. The
// composition root uses it to default a publish target to the artifact's source
// before an environment adapter is constructed.
func (m *WorkbookManager) ReadMetadata(_ context.Context, path string) (WorkbookMetadata, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return WorkbookMetadata{}, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return WorkbookMetadata{}, fmt.Errorf("inspect workbook artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return WorkbookMetadata{}, fmt.Errorf("workbook artifact selector %q must not be a symbolic link", path)
	}
	directory := absolute
	if !info.IsDir() {
		directory = filepath.Dir(absolute)
	}
	metadata, err := readMetadata(directory)
	if err != nil {
		return WorkbookMetadata{}, err
	}
	if err := validateWorkbookMetadata(metadata); err != nil {
		return WorkbookMetadata{}, err
	}
	return metadata, nil
}

func validateWorkbookMetadata(metadata WorkbookMetadata) error {
	if metadata.Kind != "workbook" {
		return errors.New("artifact metadata does not describe a workbook canonical payload")
	}
	required := []struct {
		name  string
		value string
	}{
		{name: "name", value: metadata.Name},
		{name: "tableau_id", value: metadata.TableauID},
		{name: "source_server_origin", value: metadata.SourceServerOrigin},
		{name: "source_site_luid", value: metadata.SourceSiteLUID},
		{name: "source_environment", value: metadata.SourceEnvironment},
		{name: "source_project_name", value: metadata.SourceProjectName},
		{name: "source_project_id", value: metadata.SourceProjectID},
		{name: "pulled_at", value: metadata.PulledAt},
		{name: "local_baseline_fingerprint", value: metadata.LocalBaselineFingerprint},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("workbook artifact metadata requires %s", field.name)
		}
	}
	if !metadata.sourceSitePresent {
		return errors.New("workbook artifact metadata requires source_site")
	}
	serverOrigin, err := NormalizeServerOrigin(metadata.SourceServerOrigin)
	if err != nil {
		return err
	}
	if serverOrigin != metadata.SourceServerOrigin {
		return errors.New("workbook artifact metadata requires a canonical source_server_origin")
	}
	if metadata.SourceSiteLUID != strings.TrimSpace(metadata.SourceSiteLUID) {
		return errors.New("workbook artifact metadata requires a canonical source_site_luid")
	}
	if _, err := time.Parse(time.RFC3339Nano, metadata.PulledAt); err != nil {
		return fmt.Errorf("workbook artifact metadata has invalid pulled_at: %w", err)
	}
	const prefix = "sha256:"
	digest := strings.TrimPrefix(metadata.LocalBaselineFingerprint, prefix)
	decoded, err := hex.DecodeString(digest)
	if !strings.HasPrefix(metadata.LocalBaselineFingerprint, prefix) || err != nil || len(decoded) != sha256.Size {
		return errors.New("workbook artifact metadata has invalid local_baseline_fingerprint")
	}
	if metadata.Portability != "" && metadata.Portability != PortabilityPortable && metadata.Portability != PortabilitySourceSiteBound && metadata.Portability != PortabilityUnknown {
		return fmt.Errorf("workbook artifact metadata has invalid portability %q", metadata.Portability)
	}
	if metadata.Portability == PortabilitySourceSiteBound && len(metadata.PublishedDatasources) == 0 {
		return errors.New("source-site-bound workbook artifact metadata requires published_datasources")
	}
	if len(metadata.PublishedDatasources) > 0 && metadata.Portability != PortabilitySourceSiteBound {
		return errors.New("workbook artifact metadata with published_datasources requires source-site-bound portability")
	}
	if metadata.DependenciesAcquired && len(metadata.PublishedDatasources) == 0 {
		return errors.New("workbook artifact metadata dependencies_acquired requires published_datasources")
	}
	seenLUIDs := make(map[string]struct{}, len(metadata.PublishedDatasources))
	seenPaths := make(map[string]struct{}, len(metadata.PublishedDatasources))
	previousLUID := ""
	for index, reference := range metadata.PublishedDatasources {
		luid := strings.TrimSpace(reference.LUID)
		if luid == "" {
			return fmt.Errorf("workbook artifact metadata published_datasource %d requires luid", index)
		}
		if luid != reference.LUID {
			return fmt.Errorf("workbook artifact metadata published_datasource %d requires canonical luid", index)
		}
		if _, exists := seenLUIDs[luid]; exists {
			return fmt.Errorf("workbook artifact metadata contains duplicate published datasource LUID %q", luid)
		}
		seenLUIDs[luid] = struct{}{}
		if previousLUID != "" && luid < previousLUID {
			return errors.New("workbook artifact metadata published_datasources must be sorted by LUID")
		}
		previousLUID = luid
		localPath := reference.LocalArtifactPath
		if metadata.DependenciesAcquired && localPath == "" {
			return fmt.Errorf("workbook artifact metadata published_datasource %d requires local_artifact_path when dependencies_acquired is true", index)
		}
		if !metadata.DependenciesAcquired && localPath != "" {
			return fmt.Errorf("workbook artifact metadata published_datasource %d local_artifact_path requires dependencies_acquired", index)
		}
		if localPath != "" {
			if strings.Contains(localPath, `\`) || pathpkg.IsAbs(localPath) || filepath.IsAbs(localPath) || pathpkg.Clean(localPath) != localPath || !strings.HasPrefix(localPath, "artifacts/datasource/") {
				return fmt.Errorf("workbook artifact metadata published_datasource %d has invalid local_artifact_path %q", index, localPath)
			}
			if _, exists := seenPaths[localPath]; exists {
				return fmt.Errorf("workbook artifact metadata contains duplicate local_artifact_path %q", localPath)
			}
			seenPaths[localPath] = struct{}{}
		}
	}
	return nil
}

func findBySourceIdentity(root, serverOrigin, siteLUID, workbookLUID string) (string, *WorkbookMetadata, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", nil, err
	}
	var path string
	var found *WorkbookMetadata
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tadx-") {
			continue
		}
		// A symbolic link inside the managed artifact root is a tamper or
		// misconfiguration signal: the root is owned exclusively by tadx and
		// never contains links. Refuse loudly rather than silently ignoring it,
		// since a link can alias a path outside the artifact root.
		if entry.Type()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("workbook artifact path %q escapes artifact root %q", filepath.Join(root, entry.Name()), root)
		}
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		if err := validateContainedPath(root, candidate); err != nil {
			return "", nil, err
		}
		metadata, err := readMetadata(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		if !sameSourceIdentity(metadata, serverOrigin, siteLUID, workbookLUID) {
			continue
		}
		if err := validateManagedWorkbook(candidate, metadata); err != nil {
			return "", nil, fmt.Errorf("invalid workbook artifact %q: %w", candidate, err)
		}
		if found != nil {
			return "", nil, fmt.Errorf("multiple local workbook artifacts claim one Tableau source identity")
		}
		copy := metadata
		path, found = candidate, &copy
	}
	return path, found, nil
}

const (
	backupPrefix = ".tadx-workbook-backup-"
	stagePrefix  = ".tadx-workbook-stage-"
)

// recoverWorkbookRoot reconciles the durable state left by any interrupted
// replacement before a pull consults the artifact root. A crash between the two
// renames of replaceDirectory can leave a lone `.tadx-workbook-backup-*` while
// its target is missing; because findBySourceIdentity skips every `.tadx-`
// entry, the artifact would otherwise appear permanently deleted. This sweep
// restores such a backup, garbage-collects committed backups, and removes
// staging directories abandoned by crashed pulls. It must run while the
// workspace lock is held so it cannot race a concurrent pull.
func recoverWorkbookRoot(root string, operations directoryOperations) ([]string, error) {
	operations = operations.withDefaults()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	var backups, stages []string
	for _, entry := range entries {
		name := entry.Name()
		switch {
		case strings.HasPrefix(name, backupPrefix):
			backups = append(backups, name)
			continue
		case strings.HasPrefix(name, stagePrefix):
			stages = append(stages, name)
			continue
		case strings.HasPrefix(name, ".tadx-"):
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		metadata, err := readMetadata(filepath.Join(root, name))
		if err != nil {
			continue
		}
		live[identityKey(metadata)] = true
	}
	var warnings []string
	for _, name := range stages {
		if err := operations.removeAll(filepath.Join(root, name)); err != nil {
			warnings = append(warnings, fmt.Sprintf("stale staging directory %q could not be removed: %v", name, err))
		}
	}
	changed := false
	for _, name := range backups {
		backupPath := filepath.Join(root, name)
		metadata, err := readMetadata(backupPath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("orphaned backup %q could not be read and was left in place: %v", name, err))
			continue
		}
		key := identityKey(metadata)
		if live[key] {
			if err := operations.removeAll(backupPath); err != nil {
				warnings = append(warnings, fmt.Sprintf("committed backup %q could not be removed: %v", name, err))
			}
			continue
		}
		restore := filepath.Join(root, identityComponent(metadata.Name, metadata.SourceServerOrigin, metadata.SourceSiteLUID, metadata.TableauID))
		if _, statErr := os.Lstat(restore); statErr == nil {
			warnings = append(warnings, fmt.Sprintf("orphaned backup %q could not be restored because %q already exists", name, filepath.Base(restore)))
			continue
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return warnings, statErr
		}
		if err := operations.rename(backupPath, restore); err != nil {
			return warnings, fmt.Errorf("restore orphaned workbook artifact from %q: %w", name, err)
		}
		changed = true
		live[key] = true
		warnings = append(warnings, fmt.Sprintf("recovered workbook artifact %q from an interrupted replacement", filepath.Base(restore)))
	}
	if changed {
		if err := operations.syncDir(root); err != nil {
			return warnings, fmt.Errorf("sync workbook artifact root after recovery: %w", err)
		}
	}
	return warnings, nil
}

func identityKey(metadata WorkbookMetadata) string {
	return fmt.Sprintf("%d:%s\x00%d:%s\x00%d:%s", len(metadata.SourceServerOrigin), metadata.SourceServerOrigin, len(metadata.SourceSiteLUID), metadata.SourceSiteLUID, len(metadata.TableauID), metadata.TableauID)
}

func ensureWorkbookRoot(workspace string) (string, error) {
	resolvedWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	current := workspace
	for _, name := range []string{"artifacts", "workbook"} {
		current = filepath.Join(current, name)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o700); err != nil {
				return "", fmt.Errorf("create workbook artifact root: %w", err)
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return "", fmt.Errorf("inspect workbook artifact root: %w", err)
		}
		// Refuse symlinked root components outright, matching findBySourceIdentity
		// and Read: the artifact root is owned exclusively by tadx and must be a
		// real directory. This also removes a narrow TOCTOU between the Lstat here
		// and later path resolution.
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("workbook artifact root %q must not be a symbolic link", current)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("workbook artifact root %q is not a directory", current)
		}
		resolvedCurrent, err := filepath.EvalSymlinks(current)
		if err != nil {
			return "", fmt.Errorf("resolve workbook artifact root: %w", err)
		}
		if !pathContained(resolvedWorkspace, resolvedCurrent) {
			return "", fmt.Errorf("workbook artifact root %q escapes workspace %q", current, workspace)
		}
		resolvedInfo, err := os.Stat(current)
		if err != nil {
			return "", fmt.Errorf("inspect resolved workbook artifact root: %w", err)
		}
		if !resolvedInfo.IsDir() {
			return "", fmt.Errorf("workbook artifact root %q is not a directory", current)
		}
	}
	return current, nil
}

func validateContainedPath(root, target string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve workbook artifact root: %w", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return fmt.Errorf("resolve workbook artifact path %q: %w", target, err)
	}
	if !pathContained(resolvedRoot, resolvedTarget) {
		return fmt.Errorf("workbook artifact path %q escapes artifact root %q", target, root)
	}
	return nil
}

func pathContained(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func readMetadata(directory string) (WorkbookMetadata, error) {
	path := filepath.Join(directory, "metadata.json")
	info, err := os.Lstat(path)
	if err != nil {
		return WorkbookMetadata{}, err
	}
	if !info.Mode().IsRegular() {
		return WorkbookMetadata{}, fmt.Errorf("workbook metadata in %q must be a regular file", directory)
	}
	if info.Size() > maxWorkbookMetadataBytes {
		return WorkbookMetadata{}, fmt.Errorf("workbook metadata in %q exceeds %d-byte limit", directory, maxWorkbookMetadataBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return WorkbookMetadata{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxWorkbookMetadataBytes+1))
	if err != nil {
		return WorkbookMetadata{}, fmt.Errorf("read workbook metadata in %q: %w", directory, err)
	}
	if len(data) > maxWorkbookMetadataBytes {
		return WorkbookMetadata{}, fmt.Errorf("workbook metadata in %q exceeds %d-byte limit", directory, maxWorkbookMetadataBytes)
	}
	var metadata WorkbookMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return WorkbookMetadata{}, fmt.Errorf("decode workbook metadata in %q: %w", directory, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return WorkbookMetadata{}, fmt.Errorf("decode workbook metadata fields in %q: %w", directory, err)
	}
	if rawSite, exists := fields["source_site"]; exists {
		var site *string
		if err := json.Unmarshal(rawSite, &site); err == nil && site != nil {
			metadata.sourceSitePresent = true
		}
	}
	return metadata, nil
}

func validateManagedWorkbook(directory string, metadata WorkbookMetadata) error {
	if err := validateWorkbookMetadata(metadata); err != nil {
		return err
	}
	if _, err := canonicalWorkbookPath(directory, metadata.CanonicalPayload); err != nil {
		return err
	}
	return validateWorkbookView(directory)
}

func validateWorkbookView(directory string) error {
	info, err := os.Lstat(filepath.Join(directory, "view.md"))
	if err != nil {
		return fmt.Errorf("inspect workbook view: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("workbook view must be a regular file")
	}
	return nil
}

func canonicalWorkbookPath(directory, payload string) (string, error) {
	if payload == "" || filepath.IsAbs(payload) || filepath.Base(payload) != payload || filepath.Clean(payload) != payload {
		return "", fmt.Errorf("invalid workbook canonical payload %q", payload)
	}
	extension := strings.ToLower(filepath.Ext(payload))
	if extension != ".twb" && extension != ".twbx" {
		return "", fmt.Errorf("unsupported workbook canonical payload %q", payload)
	}
	canonical := filepath.Join(directory, payload)
	info, err := os.Lstat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect canonical workbook: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("workbook canonical payload %q must not be a symbolic link", payload)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("workbook canonical payload %q must be a regular file", payload)
	}
	resolvedDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", fmt.Errorf("resolve workbook artifact directory: %w", err)
	}
	resolvedCanonical, err := filepath.EvalSymlinks(canonical)
	if err != nil {
		return "", fmt.Errorf("resolve canonical workbook: %w", err)
	}
	relative, err := filepath.Rel(resolvedDirectory, resolvedCanonical)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workbook canonical payload %q escapes its artifact directory", payload)
	}
	return canonical, nil
}

type directoryOperations struct {
	stat      func(string) (os.FileInfo, error)
	rename    func(string, string) error
	link      func(string, string) error
	removeAll func(string) error
	writeFile func(string, []byte, os.FileMode) error
	syncDir   func(string) error
}

func defaultDirectoryOperations() directoryOperations {
	return directoryOperations{
		stat:      os.Stat,
		rename:    os.Rename,
		link:      os.Link,
		removeAll: os.RemoveAll,
		writeFile: writeFileSync,
		syncDir:   fsyncDir,
	}
}

// withDefaults fills any unset hook with its production implementation so that
// tests may inject a single seam without rewiring every operation.
func (o directoryOperations) withDefaults() directoryOperations {
	if o.stat == nil {
		o.stat = os.Stat
	}
	if o.rename == nil {
		o.rename = os.Rename
	}
	if o.link == nil {
		o.link = os.Link
	}
	if o.removeAll == nil {
		o.removeAll = os.RemoveAll
	}
	if o.writeFile == nil {
		o.writeFile = writeFileSync
	}
	if o.syncDir == nil {
		o.syncDir = fsyncDir
	}
	return o
}

// writeFileSync writes data to path and flushes it to stable storage before
// returning, so a power loss cannot leave a torn or empty staged file.
func writeFileSync(path string, data []byte, perm os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// writeStagedArtifact writes each staged file durably and then flushes the
// staging directory so both file contents and directory entries survive a
// crash before the atomic rename into place.
func writeStagedArtifact(staging string, files map[string][]byte, operations directoryOperations) error {
	operations = operations.withDefaults()
	for name, data := range files {
		if err := operations.writeFile(filepath.Join(staging, name), data, 0o600); err != nil {
			return fmt.Errorf("write staged artifact %s: %w", name, err)
		}
	}
	if err := operations.syncDir(staging); err != nil {
		return fmt.Errorf("sync staged artifact directory: %w", err)
	}
	return nil
}

func replaceDirectory(staging, target string) ([]string, error) {
	return replaceDirectoryWithOperations(staging, target, defaultDirectoryOperations())
}

func replaceDirectoryWithOperations(staging, target string, operations directoryOperations) ([]string, error) {
	operations = operations.withDefaults()
	parent := filepath.Dir(target)
	if _, err := operations.stat(target); errors.Is(err, os.ErrNotExist) {
		if err := operations.rename(staging, target); err != nil {
			return nil, fmt.Errorf("install workbook artifact: %w", err)
		}
		if err := operations.syncDir(parent); err != nil {
			return nil, fmt.Errorf("sync workbook artifact root: %w", err)
		}
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	backup := filepath.Join(parent, ".tadx-workbook-backup-"+filepath.Base(target)+"-"+uniqueSuffix())
	if err := operations.rename(target, backup); err != nil {
		return nil, fmt.Errorf("stage existing workbook artifact: %w", err)
	}
	if err := operations.rename(staging, target); err != nil {
		if restoreErr := operations.rename(backup, target); restoreErr != nil {
			return nil, fmt.Errorf("install workbook artifact: %w; restore previous workbook artifact from %q: %v", err, backup, restoreErr)
		}
		return nil, fmt.Errorf("install workbook artifact: %w", err)
	}
	// Flush the parent directory so both the removal of the old entry and the
	// creation of the new one are durable before we report success. If a crash
	// occurs before this point, the recovery sweep reconciles the lone backup.
	if err := operations.syncDir(parent); err != nil {
		return nil, fmt.Errorf("sync workbook artifact root: %w", err)
	}
	if err := operations.removeAll(backup); err != nil {
		return []string{fmt.Sprintf("workbook artifact replacement committed, but backup %q could not be removed: %v", backup, err)}, nil
	}
	return nil, nil
}

// uniqueSuffix returns a collision-resistant token combining a monotonic
// timestamp with cryptographic randomness, so two replacements within the same
// nanosecond cannot produce the same backup name.
func uniqueSuffix() string {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		// crypto/rand failure is fatal for security-sensitive uniqueness; fall
		// back to the timestamp alone rather than a predictable constant.
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), hex.EncodeToString(random[:]))
}

func fingerprint(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func fingerprintFile(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	buffer := make([]byte, 1024*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			if _, err := hash.Write(buffer[:count]); err != nil {
				return "", err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
		}
		if readErr != nil {
			return "", readErr
		}
	}
}

func safeName(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, character := range value {
		switch character {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			builder.WriteByte('_')
		default:
			if character < 32 {
				builder.WriteByte('_')
			} else {
				builder.WriteRune(character)
			}
		}
	}
	result := strings.Trim(builder.String(), ". ")
	if result == "" {
		return "workbook"
	}
	return result
}

const (
	maxPortableComponentBytes = 180
	maxWorkbookMetadataBytes  = 64 * 1024
)

func identityComponent(name, serverOrigin, siteLUID, workbookLUID string) string {
	identity := fmt.Sprintf("%d:%s%d:%s%d:%s", len(serverOrigin), serverOrigin, len(siteLUID), siteLUID, len(workbookLUID), workbookLUID)
	sum := sha256.Sum256([]byte(identity))
	suffix := "--" + hex.EncodeToString(sum[:])
	return portableComponent(name, "workbook", maxPortableComponentBytes-len(suffix)) + suffix
}

func sameSourceIdentity(metadata WorkbookMetadata, serverOrigin, siteLUID, workbookLUID string) bool {
	return metadata.SourceServerOrigin == serverOrigin && metadata.SourceSiteLUID == siteLUID && metadata.TableauID == workbookLUID
}

// NormalizeServerOrigin returns the stable HTTPS origin used in workbook source identities.
func NormalizeServerOrigin(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("workbook artifact metadata requires an absolute HTTPS source_server_origin")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", errors.New("workbook artifact metadata requires an absolute HTTPS source_server_origin")
	}
	port := parsed.Port()
	if port == "443" {
		port = ""
	}
	host := hostname
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	return (&url.URL{Scheme: "https", Host: host}).String(), nil
}

func portableComponent(value, fallback string, maxBytes int) string {
	original := strings.TrimSpace(value)
	result := safeName(value)
	if result == original && len(result) <= maxBytes && !windowsReservedComponent(result) {
		return result
	}
	sum := sha256.Sum256([]byte(value))
	suffix := "-" + hex.EncodeToString(sum[:8])
	prefix := strings.TrimRight(truncateUTF8(result, maxBytes-len(suffix)), ". ")
	if prefix == "" || windowsReservedComponent(prefix+suffix) {
		prefix = fallback
	}
	return prefix + suffix
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}

func windowsReservedComponent(value string) bool {
	base := strings.ToUpper(strings.SplitN(strings.TrimRight(value, ". "), ".", 2)[0])
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	runes := []rune(base)
	if len(runes) == 4 && (string(runes[:3]) == "COM" || string(runes[:3]) == "LPT") {
		return runes[3] >= '1' && runes[3] <= '9' || runes[3] == '¹' || runes[3] == '²' || runes[3] == '³'
	}
	return false
}

func workbookView(metadata WorkbookMetadata) string {
	view := fmt.Sprintf("# %s\n\n- Kind: workbook\n- Tableau LUID: `%s`\n- Source server origin: `%s`\n- Source site LUID: `%s`\n- Source environment: `%s`\n- Source site: `%s`\n- Source project: `%s`\n- Pulled at: `%s`\n- Canonical payload: `%s`\n", metadata.Name, metadata.TableauID, metadata.SourceServerOrigin, metadata.SourceSiteLUID, metadata.SourceEnvironment, metadata.SourceSite, metadata.SourceProjectName, metadata.PulledAt, metadata.CanonicalPayload)
	if metadata.Portability != "" {
		view += fmt.Sprintf("- Portability: `%s`\n", metadata.Portability)
	}
	view += fmt.Sprintf("- Dependencies acquired: `%t`\n", metadata.DependenciesAcquired)
	if len(metadata.PublishedDatasources) > 0 {
		view += "\n## Published datasource references\n\n"
		for _, reference := range metadata.PublishedDatasources {
			view += fmt.Sprintf("- `%s` %s (source site `%s`)", reference.LUID, reference.Name, reference.SourceSite)
			if reference.LocalArtifactPath != "" {
				view += fmt.Sprintf(" -> `%s`", reference.LocalArtifactPath)
			}
			view += "\n"
		}
	}
	return view
}
