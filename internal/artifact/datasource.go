package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// CompositionStatusUnknown records that TADX preserved the datasource package
	// without attempting to classify or rewrite its internal composition.
	CompositionStatusUnknown = "unknown"

	maxDatasourceMetadataBytes = 64 * 1024
	datasourceBackupPrefix     = ".tadx-datasource-backup-"
	datasourceStagePrefix      = ".tadx-datasource-stage-"
)

// DatasourceMetadata is the persisted provenance for one native datasource.
type DatasourceMetadata struct {
	Kind                     string `json:"kind"`
	Name                     string `json:"name"`
	TableauID                string `json:"tableau_id"`
	SourceServerOrigin       string `json:"source_server_origin"`
	SourceSiteLUID           string `json:"source_site_luid"`
	SourceEnvironment        string `json:"source_environment"`
	SourceSite               string `json:"source_site"`
	SourceProjectName        string `json:"source_project_name"`
	SourceProjectID          string `json:"source_project_id"`
	PulledAt                 string `json:"pulled_at"`
	CanonicalPayload         string `json:"canonical_payload"`
	LocalBaselineFingerprint string `json:"local_baseline_fingerprint"`
	CompositionStatus        string `json:"composition_status"`
	sourceSitePresent        bool
}

// DatasourcePull describes one complete datasource artifact replacement.
type DatasourcePull struct {
	Workspace string
	Filename  string
	Content   []byte
	Metadata  DatasourceMetadata
	Overwrite bool
}

// DatasourcePullResult identifies the materialized datasource artifact.
type DatasourcePullResult struct {
	ArtifactPath          string
	CanonicalPath         string
	WorkspaceRelativePath string
	BaselineFingerprint   string
	Warnings              []string
}

// DatasourceArtifact is the validated native payload and its provenance.
type DatasourceArtifact struct {
	Path               string
	PayloadPath        string
	Filename           string
	Size               int64
	Name               string
	TableauID          string
	Fingerprint        string
	SourceServerOrigin string
	SourceSiteLUID     string
	SourceEnvironment  string
	SourceSite         string
	SourceProjectName  string
	SourceProjectID    string
	CompositionStatus  string
}

// DatasourceManager owns first-class datasource artifact persistence.
type DatasourceManager struct {
	now        func() time.Time
	operations directoryOperations
}

// NewDatasourceManager creates a datasource artifact manager.
func NewDatasourceManager(now func() time.Time) *DatasourceManager {
	if now == nil {
		now = time.Now
	}
	return &DatasourceManager{now: now, operations: defaultDirectoryOperations()}
}

// Pull creates or safely refreshes one identity-matched datasource artifact.
func (m *DatasourceManager) Pull(ctx context.Context, input DatasourcePull) (DatasourcePullResult, error) {
	if err := ctx.Err(); err != nil {
		return DatasourcePullResult{}, err
	}
	if strings.TrimSpace(input.Workspace) == "" || strings.TrimSpace(input.Metadata.TableauID) == "" || strings.TrimSpace(input.Metadata.Name) == "" {
		return DatasourcePullResult{}, errors.New("datasource artifact requires workspace, Tableau ID, and name")
	}
	origin, err := NormalizeServerOrigin(input.Metadata.SourceServerOrigin)
	if err != nil {
		return DatasourcePullResult{}, errors.New(strings.NewReplacer("workbook artifact", "datasource artifact").Replace(err.Error()))
	}
	input.Metadata.SourceServerOrigin = origin
	input.Metadata.SourceSiteLUID = strings.TrimSpace(input.Metadata.SourceSiteLUID)
	if input.Metadata.SourceSiteLUID == "" {
		return DatasourcePullResult{}, errors.New("datasource artifact metadata requires source_site_luid")
	}
	nativeName := filepath.Base(input.Filename)
	nativeExtension := filepath.Ext(nativeName)
	extension := strings.ToLower(nativeExtension)
	if extension != ".tds" && extension != ".tdsx" {
		return DatasourcePullResult{}, fmt.Errorf("unsupported datasource artifact filename %q", input.Filename)
	}
	filename := portableComponent(strings.TrimSuffix(nativeName, nativeExtension), "datasource", maxPortableComponentBytes-len(nativeExtension)) + nativeExtension
	workspace, err := filepath.Abs(input.Workspace)
	if err != nil {
		return DatasourcePullResult{}, err
	}
	if _, err := os.Stat(filepath.Join(workspace, "tadx.yaml")); err != nil {
		return DatasourcePullResult{}, fmt.Errorf("workspace %q does not contain tadx.yaml", workspace)
	}
	root, err := ensureDatasourceRoot(workspace)
	if err != nil {
		return DatasourcePullResult{}, err
	}
	operations := m.operations.withDefaults()
	warnings, err := recoverDatasourceRoot(root, operations)
	if err != nil {
		return DatasourcePullResult{}, err
	}
	target, existing, err := findDatasourceBySourceIdentity(root, origin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID)
	if err != nil {
		return DatasourcePullResult{}, err
	}
	if target == "" {
		target = filepath.Join(root, datasourceIdentityComponent(input.Metadata.Name, origin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID))
		if info, statErr := os.Lstat(target); statErr == nil {
			if err := validateDatasourceContainedPath(root, target); err != nil {
				return DatasourcePullResult{}, err
			}
			if !info.IsDir() {
				return DatasourcePullResult{}, fmt.Errorf("artifact path %q is not a managed datasource directory", target)
			}
			metadata, readErr := readDatasourceMetadata(target)
			if readErr != nil {
				return DatasourcePullResult{}, fmt.Errorf("artifact path %q is not a managed datasource artifact: %w", target, readErr)
			}
			if !sameDatasourceSourceIdentity(metadata, origin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID) {
				return DatasourcePullResult{}, fmt.Errorf("artifact path %q belongs to a different Tableau source identity", target)
			}
			if err := validateManagedDatasource(target, metadata); err != nil {
				return DatasourcePullResult{}, fmt.Errorf("artifact path %q is not a managed datasource artifact: %w", target, err)
			}
			existing = &metadata
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return DatasourcePullResult{}, statErr
		}
	}
	if existing != nil {
		canonical, err := canonicalDatasourcePath(target, existing.CanonicalPayload)
		if err != nil {
			return DatasourcePullResult{}, err
		}
		current, err := fingerprintFile(ctx, canonical)
		if err != nil {
			return DatasourcePullResult{}, fmt.Errorf("fingerprint existing canonical datasource: %w", err)
		}
		dirty := current != existing.LocalBaselineFingerprint
		if dirty && !input.Overwrite {
			return DatasourcePullResult{}, fmt.Errorf("datasource artifact %q is dirty; use --overwrite to replace local edits", target)
		}
		if dirty {
			warnings = append(warnings, "dirty datasource artifact replaced because --overwrite was provided")
		} else {
			warnings = append(warnings, "clean datasource artifact refreshed from Tableau")
		}
	}

	baseline := fingerprint(input.Content)
	metadata := input.Metadata
	metadata.Kind = "datasource"
	metadata.PulledAt = m.now().UTC().Format(time.RFC3339Nano)
	metadata.CanonicalPayload = filename
	metadata.LocalBaselineFingerprint = baseline
	metadata.CompositionStatus = CompositionStatusUnknown
	metadata.sourceSitePresent = true
	if err := validateDatasourceMetadata(metadata); err != nil {
		return DatasourcePullResult{}, err
	}
	metadataBytes, err := encodeDatasourceMetadata(metadata)
	if err != nil {
		return DatasourcePullResult{}, err
	}
	staging, err := os.MkdirTemp(root, datasourceStagePrefix)
	if err != nil {
		return DatasourcePullResult{}, fmt.Errorf("create datasource artifact staging directory: %w", err)
	}
	defer os.RemoveAll(staging)
	files := map[string][]byte{
		filename:        input.Content,
		"metadata.json": metadataBytes,
		"view.md":       []byte(datasourceView(metadata)),
	}
	if err := writeStagedArtifact(staging, files, operations); err != nil {
		return DatasourcePullResult{}, err
	}
	replaceWarnings, err := replaceDatasourceDirectory(staging, target, operations)
	if err != nil {
		return DatasourcePullResult{}, err
	}
	warnings = append(warnings, replaceWarnings...)
	relative, err := filepath.Rel(workspace, target)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return DatasourcePullResult{}, fmt.Errorf("datasource artifact path %q escapes workspace %q", target, workspace)
	}
	return DatasourcePullResult{
		ArtifactPath:          target,
		CanonicalPath:         filepath.Join(target, filename),
		WorkspaceRelativePath: filepath.ToSlash(relative),
		BaselineFingerprint:   baseline,
		Warnings:              warnings,
	}, nil
}

// Read validates a datasource artifact and returns its current native payload.
func (m *DatasourceManager) Read(ctx context.Context, path string) (DatasourceArtifact, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return DatasourceArtifact{}, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return DatasourceArtifact{}, fmt.Errorf("inspect datasource artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return DatasourceArtifact{}, fmt.Errorf("datasource artifact selector %q must not be a symbolic link", path)
	}
	directory := absolute
	if !info.IsDir() {
		directory = filepath.Dir(absolute)
	}
	metadata, err := readDatasourceMetadata(directory)
	if err != nil {
		return DatasourceArtifact{}, err
	}
	if err := validateDatasourceMetadata(metadata); err != nil {
		return DatasourceArtifact{}, err
	}
	if err := validateDatasourceView(directory); err != nil {
		return DatasourceArtifact{}, err
	}
	canonical, err := canonicalDatasourcePath(directory, metadata.CanonicalPayload)
	if err != nil {
		return DatasourceArtifact{}, err
	}
	canonicalInfo, err := os.Stat(canonical)
	if err != nil {
		return DatasourceArtifact{}, fmt.Errorf("inspect canonical datasource: %w", err)
	}
	if !info.IsDir() && !os.SameFile(info, canonicalInfo) {
		return DatasourceArtifact{}, fmt.Errorf("datasource artifact file %q is not the canonical payload %q", path, metadata.CanonicalPayload)
	}
	current, err := fingerprintFile(ctx, canonical)
	if err != nil {
		return DatasourceArtifact{}, fmt.Errorf("fingerprint canonical datasource: %w", err)
	}
	return DatasourceArtifact{
		Path: directory, PayloadPath: canonical, Filename: metadata.CanonicalPayload, Size: canonicalInfo.Size(),
		Name: metadata.Name, TableauID: metadata.TableauID, Fingerprint: current,
		SourceServerOrigin: metadata.SourceServerOrigin, SourceSiteLUID: metadata.SourceSiteLUID,
		SourceEnvironment: metadata.SourceEnvironment, SourceSite: metadata.SourceSite,
		SourceProjectName: metadata.SourceProjectName, SourceProjectID: metadata.SourceProjectID,
		CompositionStatus: metadata.CompositionStatus,
	}, nil
}

// ReadMetadata returns validated datasource provenance without reading payload bytes.
func (m *DatasourceManager) ReadMetadata(_ context.Context, path string) (DatasourceMetadata, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return DatasourceMetadata{}, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return DatasourceMetadata{}, fmt.Errorf("inspect datasource artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return DatasourceMetadata{}, fmt.Errorf("datasource artifact selector %q must not be a symbolic link", path)
	}
	directory := absolute
	if !info.IsDir() {
		directory = filepath.Dir(absolute)
	}
	metadata, err := readDatasourceMetadata(directory)
	if err != nil {
		return DatasourceMetadata{}, err
	}
	if err := validateDatasourceMetadata(metadata); err != nil {
		return DatasourceMetadata{}, err
	}
	return metadata, nil
}

func encodeDatasourceMetadata(metadata DatasourceMetadata) ([]byte, error) {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode datasource metadata: %w", err)
	}
	data = append(data, '\n')
	if len(data) > maxDatasourceMetadataBytes {
		return nil, fmt.Errorf("datasource metadata exceeds %d-byte limit", maxDatasourceMetadataBytes)
	}
	return data, nil
}

func readDatasourceMetadata(directory string) (DatasourceMetadata, error) {
	path := filepath.Join(directory, "metadata.json")
	info, err := os.Lstat(path)
	if err != nil {
		return DatasourceMetadata{}, err
	}
	if !info.Mode().IsRegular() {
		return DatasourceMetadata{}, fmt.Errorf("datasource metadata in %q must be a regular file", directory)
	}
	if info.Size() > maxDatasourceMetadataBytes {
		return DatasourceMetadata{}, fmt.Errorf("datasource metadata in %q exceeds %d-byte limit", directory, maxDatasourceMetadataBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return DatasourceMetadata{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxDatasourceMetadataBytes+1))
	if err != nil {
		return DatasourceMetadata{}, fmt.Errorf("read datasource metadata in %q: %w", directory, err)
	}
	if len(data) > maxDatasourceMetadataBytes {
		return DatasourceMetadata{}, fmt.Errorf("datasource metadata in %q exceeds %d-byte limit", directory, maxDatasourceMetadataBytes)
	}
	var metadata DatasourceMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return DatasourceMetadata{}, fmt.Errorf("decode datasource metadata in %q: %w", directory, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return DatasourceMetadata{}, fmt.Errorf("decode datasource metadata fields in %q: %w", directory, err)
	}
	if raw, ok := fields["source_site"]; ok {
		var value *string
		if err := json.Unmarshal(raw, &value); err == nil && value != nil {
			metadata.sourceSitePresent = true
		}
	}
	return metadata, nil
}

func validateDatasourceMetadata(metadata DatasourceMetadata) error {
	if metadata.Kind != "datasource" {
		return errors.New("artifact metadata does not describe a datasource canonical payload")
	}
	required := []struct{ name, value string }{
		{"name", metadata.Name}, {"tableau_id", metadata.TableauID}, {"source_server_origin", metadata.SourceServerOrigin},
		{"source_site_luid", metadata.SourceSiteLUID}, {"source_environment", metadata.SourceEnvironment},
		{"source_project_name", metadata.SourceProjectName}, {"source_project_id", metadata.SourceProjectID},
		{"pulled_at", metadata.PulledAt}, {"canonical_payload", metadata.CanonicalPayload},
		{"local_baseline_fingerprint", metadata.LocalBaselineFingerprint}, {"composition_status", metadata.CompositionStatus},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("datasource artifact metadata requires %s", field.name)
		}
	}
	if !metadata.sourceSitePresent {
		return errors.New("datasource artifact metadata requires source_site")
	}
	origin, err := NormalizeServerOrigin(metadata.SourceServerOrigin)
	if err != nil || origin != metadata.SourceServerOrigin {
		return errors.New("datasource artifact metadata requires a canonical source_server_origin")
	}
	if metadata.SourceSiteLUID != strings.TrimSpace(metadata.SourceSiteLUID) {
		return errors.New("datasource artifact metadata requires a canonical source_site_luid")
	}
	if _, err := time.Parse(time.RFC3339Nano, metadata.PulledAt); err != nil {
		return fmt.Errorf("datasource artifact metadata has invalid pulled_at: %w", err)
	}
	digest := strings.TrimPrefix(metadata.LocalBaselineFingerprint, "sha256:")
	decoded, err := hex.DecodeString(digest)
	if !strings.HasPrefix(metadata.LocalBaselineFingerprint, "sha256:") || err != nil || len(decoded) != sha256.Size {
		return errors.New("datasource artifact metadata has invalid local_baseline_fingerprint")
	}
	if metadata.CompositionStatus != CompositionStatusUnknown {
		return fmt.Errorf("datasource artifact metadata has invalid composition_status %q", metadata.CompositionStatus)
	}
	return nil
}

func ensureDatasourceRoot(workspace string) (string, error) {
	resolvedWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	current := workspace
	for _, name := range []string{"artifacts", "datasource"} {
		current = filepath.Join(current, name)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o700); err != nil {
				return "", fmt.Errorf("create datasource artifact root: %w", err)
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return "", fmt.Errorf("inspect datasource artifact root: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("datasource artifact root %q must not be a symbolic link", current)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("datasource artifact root %q is not a directory", current)
		}
		resolvedCurrent, err := filepath.EvalSymlinks(current)
		if err != nil {
			return "", fmt.Errorf("resolve datasource artifact root: %w", err)
		}
		if !pathContained(resolvedWorkspace, resolvedCurrent) {
			return "", fmt.Errorf("datasource artifact root %q escapes workspace %q", current, workspace)
		}
	}
	return current, nil
}

func findDatasourceBySourceIdentity(root, origin, siteLUID, datasourceLUID string) (string, *DatasourceMetadata, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", nil, err
	}
	var path string
	var found *DatasourceMetadata
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tadx-") {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("datasource artifact path %q escapes artifact root %q", candidate, root)
		}
		if !entry.IsDir() {
			continue
		}
		if err := validateDatasourceContainedPath(root, candidate); err != nil {
			return "", nil, err
		}
		metadata, err := readDatasourceMetadata(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		if !sameDatasourceSourceIdentity(metadata, origin, siteLUID, datasourceLUID) {
			continue
		}
		if err := validateManagedDatasource(candidate, metadata); err != nil {
			return "", nil, fmt.Errorf("invalid datasource artifact %q: %w", candidate, err)
		}
		if found != nil {
			return "", nil, errors.New("multiple local datasource artifacts claim one Tableau source identity")
		}
		copy := metadata
		path, found = candidate, &copy
	}
	return path, found, nil
}

func validateManagedDatasource(directory string, metadata DatasourceMetadata) error {
	if err := validateDatasourceMetadata(metadata); err != nil {
		return err
	}
	if _, err := canonicalDatasourcePath(directory, metadata.CanonicalPayload); err != nil {
		return err
	}
	return validateDatasourceView(directory)
}

func validateDatasourceContainedPath(root, target string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve datasource artifact root: %w", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return fmt.Errorf("resolve datasource artifact path %q: %w", target, err)
	}
	if !pathContained(resolvedRoot, resolvedTarget) {
		return fmt.Errorf("datasource artifact path %q escapes artifact root %q", target, root)
	}
	return nil
}

func canonicalDatasourcePath(directory, payload string) (string, error) {
	if payload == "" || filepath.IsAbs(payload) || filepath.Base(payload) != payload || filepath.Clean(payload) != payload {
		return "", fmt.Errorf("invalid datasource canonical payload %q", payload)
	}
	extension := strings.ToLower(filepath.Ext(payload))
	if extension != ".tds" && extension != ".tdsx" {
		return "", fmt.Errorf("unsupported datasource canonical payload %q", payload)
	}
	canonical := filepath.Join(directory, payload)
	info, err := os.Lstat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect canonical datasource: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("datasource canonical payload %q must not be a symbolic link", payload)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("datasource canonical payload %q must be a regular file", payload)
	}
	resolvedDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", fmt.Errorf("resolve datasource artifact directory: %w", err)
	}
	resolvedCanonical, err := filepath.EvalSymlinks(canonical)
	if err != nil {
		return "", fmt.Errorf("resolve canonical datasource: %w", err)
	}
	if !pathContained(resolvedDirectory, resolvedCanonical) {
		return "", fmt.Errorf("datasource canonical payload %q escapes its artifact directory", payload)
	}
	return canonical, nil
}

func validateDatasourceView(directory string) error {
	info, err := os.Lstat(filepath.Join(directory, "view.md"))
	if err != nil {
		return fmt.Errorf("inspect datasource view: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("datasource view must be a regular file")
	}
	return nil
}

func datasourceIdentityComponent(name, origin, siteLUID, datasourceLUID string) string {
	identity := datasourceIdentityKey(origin, siteLUID, datasourceLUID)
	sum := sha256.Sum256([]byte(identity))
	suffix := "--" + hex.EncodeToString(sum[:])
	return portableComponent(name, "datasource", maxPortableComponentBytes-len(suffix)) + suffix
}

func datasourceIdentityKey(origin, siteLUID, datasourceLUID string) string {
	return fmt.Sprintf("%d:%s\x00%d:%s\x00%d:%s", len(origin), origin, len(siteLUID), siteLUID, len(datasourceLUID), datasourceLUID)
}

func sameDatasourceSourceIdentity(metadata DatasourceMetadata, origin, siteLUID, datasourceLUID string) bool {
	return metadata.SourceServerOrigin == origin && metadata.SourceSiteLUID == siteLUID && metadata.TableauID == datasourceLUID
}

func replaceDatasourceDirectory(staging, target string, operations directoryOperations) ([]string, error) {
	operations = operations.withDefaults()
	parent := filepath.Dir(target)
	if _, err := operations.stat(target); errors.Is(err, os.ErrNotExist) {
		if err := operations.rename(staging, target); err != nil {
			return nil, fmt.Errorf("install datasource artifact: %w", err)
		}
		if err := operations.syncDir(parent); err != nil {
			return nil, fmt.Errorf("sync datasource artifact root: %w", err)
		}
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	backup := filepath.Join(parent, datasourceBackupPrefix+filepath.Base(target)+"-"+uniqueSuffix())
	if err := operations.rename(target, backup); err != nil {
		return nil, fmt.Errorf("stage existing datasource artifact: %w", err)
	}
	if err := operations.rename(staging, target); err != nil {
		if restoreErr := operations.rename(backup, target); restoreErr != nil {
			return nil, fmt.Errorf("install datasource artifact: %w; restore previous datasource artifact from %q: %v", err, backup, restoreErr)
		}
		return nil, fmt.Errorf("install datasource artifact: %w", err)
	}
	if err := operations.syncDir(parent); err != nil {
		return nil, fmt.Errorf("sync datasource artifact root: %w", err)
	}
	if err := operations.removeAll(backup); err != nil {
		return []string{fmt.Sprintf("datasource artifact replacement committed, but backup %q could not be removed: %v", backup, err)}, nil
	}
	return nil, nil
}

func recoverDatasourceRoot(root string, operations directoryOperations) ([]string, error) {
	operations = operations.withDefaults()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	var backups, stages []string
	for _, entry := range entries {
		switch {
		case strings.HasPrefix(entry.Name(), datasourceBackupPrefix):
			backups = append(backups, entry.Name())
		case strings.HasPrefix(entry.Name(), datasourceStagePrefix):
			stages = append(stages, entry.Name())
		case strings.HasPrefix(entry.Name(), ".tadx-"):
			continue
		case entry.IsDir() && entry.Type()&os.ModeSymlink == 0:
			metadata, err := readDatasourceMetadata(filepath.Join(root, entry.Name()))
			if err == nil {
				live[datasourceIdentityKey(metadata.SourceServerOrigin, metadata.SourceSiteLUID, metadata.TableauID)] = true
			}
		}
	}
	var warnings []string
	for _, name := range stages {
		if err := operations.removeAll(filepath.Join(root, name)); err != nil {
			warnings = append(warnings, fmt.Sprintf("stale datasource staging directory %q could not be removed: %v", name, err))
		}
	}
	changed := false
	for _, name := range backups {
		backup := filepath.Join(root, name)
		metadata, err := readDatasourceMetadata(backup)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("orphaned datasource backup %q could not be read and was left in place: %v", name, err))
			continue
		}
		key := datasourceIdentityKey(metadata.SourceServerOrigin, metadata.SourceSiteLUID, metadata.TableauID)
		if live[key] {
			if err := operations.removeAll(backup); err != nil {
				warnings = append(warnings, fmt.Sprintf("committed datasource backup %q could not be removed: %v", name, err))
			}
			continue
		}
		target := filepath.Join(root, datasourceIdentityComponent(metadata.Name, metadata.SourceServerOrigin, metadata.SourceSiteLUID, metadata.TableauID))
		if _, err := os.Lstat(target); err == nil {
			warnings = append(warnings, fmt.Sprintf("orphaned datasource backup %q could not be restored because %q already exists", name, filepath.Base(target)))
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return warnings, err
		}
		if err := operations.rename(backup, target); err != nil {
			return warnings, fmt.Errorf("restore orphaned datasource artifact from %q: %w", name, err)
		}
		changed = true
		live[key] = true
		warnings = append(warnings, fmt.Sprintf("recovered datasource artifact %q from an interrupted replacement", filepath.Base(target)))
	}
	if changed {
		if err := operations.syncDir(root); err != nil {
			return warnings, fmt.Errorf("sync datasource artifact root after recovery: %w", err)
		}
	}
	return warnings, nil
}

func datasourceView(metadata DatasourceMetadata) string {
	return fmt.Sprintf("# %s\n\n- Kind: datasource\n- Tableau LUID: `%s`\n- Source server origin: `%s`\n- Source site LUID: `%s`\n- Source environment: `%s`\n- Source site: `%s`\n- Source project: `%s`\n- Pulled at: `%s`\n- Canonical payload: `%s`\n- Composition status: `%s`\n", metadata.Name, metadata.TableauID, metadata.SourceServerOrigin, metadata.SourceSiteLUID, metadata.SourceEnvironment, metadata.SourceSite, metadata.SourceProjectName, metadata.PulledAt, metadata.CanonicalPayload, metadata.CompositionStatus)
}
