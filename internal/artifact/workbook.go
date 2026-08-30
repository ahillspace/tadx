// Package artifact owns deterministic local artifact persistence and dirty guards.
package artifact

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
)

// WorkbookMetadata is the frozen workbook provenance contract.
type WorkbookMetadata struct {
	Kind                     string `json:"kind"`
	Name                     string `json:"name"`
	TableauID                string `json:"tableau_id"`
	SourceEnvironment        string `json:"source_environment,omitempty"`
	SourceSite               string `json:"source_site,omitempty"`
	SourceProjectName        string `json:"source_project_name,omitempty"`
	SourceProjectID          string `json:"source_project_id,omitempty"`
	PulledAt                 string `json:"pulled_at"`
	CanonicalPayload         string `json:"canonical_payload"`
	LocalBaselineFingerprint string `json:"local_baseline_fingerprint"`
}

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
	Path        string
	Filename    string
	Content     []byte
	Name        string
	TableauID   string
	Fingerprint string
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
	filename := filepath.Base(input.Filename)
	extension := strings.ToLower(filepath.Ext(filename))
	if extension != ".twb" && extension != ".twbx" {
		return WorkbookPullResult{}, fmt.Errorf("unsupported workbook artifact filename %q", input.Filename)
	}
	workspace, err := filepath.Abs(input.Workspace)
	if err != nil {
		return WorkbookPullResult{}, err
	}
	if _, err := os.Stat(filepath.Join(workspace, "tadx.yaml")); err != nil {
		return WorkbookPullResult{}, fmt.Errorf("workspace %q does not contain tadx.yaml", workspace)
	}
	root := filepath.Join(workspace, "artifacts", "workbook")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return WorkbookPullResult{}, fmt.Errorf("create workbook artifact root: %w", err)
	}
	target, existing, err := findByTableauID(root, input.Metadata.TableauID)
	if err != nil {
		return WorkbookPullResult{}, err
	}
	if target == "" {
		target = filepath.Join(root, safeName(input.Metadata.Name))
		if info, statErr := os.Lstat(target); statErr == nil {
			if !info.IsDir() {
				return WorkbookPullResult{}, fmt.Errorf("artifact path %q is not a managed workbook directory", target)
			}
			metadata, readErr := readMetadata(target)
			if readErr != nil {
				return WorkbookPullResult{}, fmt.Errorf("artifact path %q is not a managed workbook artifact: %w", target, readErr)
			}
			if metadata.TableauID != input.Metadata.TableauID {
				return WorkbookPullResult{}, fmt.Errorf("artifact path %q belongs to Tableau ID %q", target, metadata.TableauID)
			}
			existing = &metadata
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return WorkbookPullResult{}, statErr
		}
	}
	var warnings []string
	if existing != nil {
		canonical, err := canonicalWorkbookPath(target, existing.CanonicalPayload)
		if err != nil {
			return WorkbookPullResult{}, err
		}
		current, err := os.ReadFile(canonical)
		if err != nil {
			return WorkbookPullResult{}, fmt.Errorf("read existing canonical workbook: %w", err)
		}
		dirty := fingerprint(current) != existing.LocalBaselineFingerprint
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
	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return WorkbookPullResult{}, fmt.Errorf("encode workbook metadata: %w", err)
	}
	metadataBytes = append(metadataBytes, '\n')
	view := workbookView(metadata)
	staging, err := os.MkdirTemp(root, ".tadx-workbook-stage-")
	if err != nil {
		return WorkbookPullResult{}, fmt.Errorf("create artifact staging directory: %w", err)
	}
	defer os.RemoveAll(staging)
	for name, data := range map[string][]byte{filename: input.Content, "metadata.json": metadataBytes, "view.md": []byte(view)} {
		if err := os.WriteFile(filepath.Join(staging, name), data, 0o600); err != nil {
			return WorkbookPullResult{}, fmt.Errorf("write staged artifact %s: %w", name, err)
		}
	}
	replacementWarnings, err := replaceDirectory(staging, target)
	if err != nil {
		return WorkbookPullResult{}, err
	}
	warnings = append(warnings, replacementWarnings...)
	return WorkbookPullResult{ArtifactPath: target, CanonicalPath: filepath.Join(target, filename), BaselineFingerprint: baseline, Warnings: warnings}, nil
}

// Read loads the current native workbook bytes without treating external edits as an error.
func (m *WorkbookManager) Read(_ context.Context, path string) (WorkbookArtifact, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return WorkbookArtifact{}, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return WorkbookArtifact{}, fmt.Errorf("inspect workbook artifact: %w", err)
	}
	directory := absolute
	if !info.IsDir() {
		directory = filepath.Dir(absolute)
	}
	metadata, err := readMetadata(directory)
	if err != nil {
		return WorkbookArtifact{}, err
	}
	if metadata.Kind != "workbook" {
		return WorkbookArtifact{}, errors.New("artifact metadata does not describe a workbook canonical payload")
	}
	canonical, err := canonicalWorkbookPath(directory, metadata.CanonicalPayload)
	if err != nil {
		return WorkbookArtifact{}, err
	}
	content, err := os.ReadFile(canonical)
	if err != nil {
		return WorkbookArtifact{}, fmt.Errorf("read canonical workbook: %w", err)
	}
	return WorkbookArtifact{Path: directory, Filename: metadata.CanonicalPayload, Content: content, Name: metadata.Name, TableauID: metadata.TableauID, Fingerprint: fingerprint(content)}, nil
}

func findByTableauID(root, id string) (string, *WorkbookMetadata, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", nil, err
	}
	var path string
	var found *WorkbookMetadata
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".tadx-") {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		metadata, err := readMetadata(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		if metadata.TableauID != id {
			continue
		}
		if found != nil {
			return "", nil, fmt.Errorf("multiple local workbook artifacts claim Tableau ID %q", id)
		}
		copy := metadata
		path, found = candidate, &copy
	}
	return path, found, nil
}

func readMetadata(directory string) (WorkbookMetadata, error) {
	data, err := os.ReadFile(filepath.Join(directory, "metadata.json"))
	if err != nil {
		return WorkbookMetadata{}, err
	}
	var metadata WorkbookMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return WorkbookMetadata{}, fmt.Errorf("decode workbook metadata in %q: %w", directory, err)
	}
	return metadata, nil
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
	removeAll func(string) error
}

func replaceDirectory(staging, target string) ([]string, error) {
	return replaceDirectoryWithOperations(staging, target, directoryOperations{stat: os.Stat, rename: os.Rename, removeAll: os.RemoveAll})
}

func replaceDirectoryWithOperations(staging, target string, operations directoryOperations) ([]string, error) {
	if _, err := operations.stat(target); errors.Is(err, os.ErrNotExist) {
		if err := operations.rename(staging, target); err != nil {
			return nil, fmt.Errorf("install workbook artifact: %w", err)
		}
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	backup := filepath.Join(filepath.Dir(target), ".tadx-workbook-backup-"+filepath.Base(target)+"-"+strconvTimestamp())
	if err := operations.rename(target, backup); err != nil {
		return nil, fmt.Errorf("stage existing workbook artifact: %w", err)
	}
	if err := operations.rename(staging, target); err != nil {
		if restoreErr := operations.rename(backup, target); restoreErr != nil {
			return nil, fmt.Errorf("install workbook artifact: %w; restore previous workbook artifact from %q: %v", err, backup, restoreErr)
		}
		return nil, fmt.Errorf("install workbook artifact: %w", err)
	}
	if err := operations.removeAll(backup); err != nil {
		return []string{fmt.Sprintf("workbook artifact replacement committed, but backup %q could not be removed: %v", backup, err)}, nil
	}
	return nil, nil
}

func strconvTimestamp() string { return fmt.Sprintf("%d", time.Now().UnixNano()) }

func fingerprint(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
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

func workbookView(metadata WorkbookMetadata) string {
	return fmt.Sprintf("# %s\n\n- Kind: workbook\n- Tableau LUID: `%s`\n- Source environment: `%s`\n- Source site: `%s`\n- Source project: `%s`\n- Pulled at: `%s`\n- Canonical payload: `%s`\n", metadata.Name, metadata.TableauID, metadata.SourceEnvironment, metadata.SourceSite, metadata.SourceProjectName, metadata.PulledAt, metadata.CanonicalPayload)
}
