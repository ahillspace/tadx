// Package artifact owns deterministic local artifact persistence and dirty guards.
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
	"unicode/utf8"
)

// WorkbookMetadata is the frozen workbook provenance contract.
type WorkbookMetadata struct {
	Kind                     string `json:"kind"`
	Name                     string `json:"name"`
	TableauID                string `json:"tableau_id"`
	SourceEnvironment        string `json:"source_environment,omitempty"`
	SourceSite               string `json:"source_site"`
	SourceProjectName        string `json:"source_project_name,omitempty"`
	SourceProjectID          string `json:"source_project_id,omitempty"`
	PulledAt                 string `json:"pulled_at"`
	CanonicalPayload         string `json:"canonical_payload"`
	LocalBaselineFingerprint string `json:"local_baseline_fingerprint"`
	sourceSitePresent        bool
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
	PayloadPath string
	Filename    string
	Size        int64
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
	root, err := ensureWorkbookRoot(workspace)
	if err != nil {
		return WorkbookPullResult{}, err
	}
	target, existing, err := findByTableauID(root, input.Metadata.TableauID)
	if err != nil {
		return WorkbookPullResult{}, err
	}
	if target == "" {
		target = filepath.Join(root, identityComponent(input.Metadata.Name, input.Metadata.TableauID))
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
			if metadata.TableauID != input.Metadata.TableauID {
				return WorkbookPullResult{}, fmt.Errorf("artifact path %q belongs to Tableau ID %q", target, metadata.TableauID)
			}
			if err := validateManagedWorkbook(target, metadata); err != nil {
				return WorkbookPullResult{}, fmt.Errorf("artifact path %q is not a managed workbook artifact: %w", target, err)
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

// Read validates the current native workbook and returns its publishable file contract.
func (m *WorkbookManager) Read(ctx context.Context, path string) (WorkbookArtifact, error) {
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
	if !info.IsDir() && !samePath(absolute, canonical) {
		return WorkbookArtifact{}, fmt.Errorf("workbook artifact file %q is not the canonical payload %q", path, metadata.CanonicalPayload)
	}
	canonicalInfo, err := os.Stat(canonical)
	if err != nil {
		return WorkbookArtifact{}, fmt.Errorf("inspect canonical workbook: %w", err)
	}
	currentFingerprint, err := fingerprintFile(ctx, canonical)
	if err != nil {
		return WorkbookArtifact{}, fmt.Errorf("fingerprint canonical workbook: %w", err)
	}
	return WorkbookArtifact{Path: directory, PayloadPath: canonical, Filename: metadata.CanonicalPayload, Size: canonicalInfo.Size(), Name: metadata.Name, TableauID: metadata.TableauID, Fingerprint: currentFingerprint}, nil
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
	if _, err := time.Parse(time.RFC3339Nano, metadata.PulledAt); err != nil {
		return fmt.Errorf("workbook artifact metadata has invalid pulled_at: %w", err)
	}
	const prefix = "sha256:"
	digest := strings.TrimPrefix(metadata.LocalBaselineFingerprint, prefix)
	decoded, err := hex.DecodeString(digest)
	if !strings.HasPrefix(metadata.LocalBaselineFingerprint, prefix) || err != nil || len(decoded) != sha256.Size {
		return errors.New("workbook artifact metadata has invalid local_baseline_fingerprint")
	}
	return nil
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if filepath.Separator == '\\' {
		return strings.EqualFold(left, right)
	}
	return left == right
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
		if metadata.TableauID != id {
			continue
		}
		if err := validateManagedWorkbook(candidate, metadata); err != nil {
			return "", nil, fmt.Errorf("invalid workbook artifact %q: %w", candidate, err)
		}
		if found != nil {
			return "", nil, fmt.Errorf("multiple local workbook artifacts claim Tableau ID %q", id)
		}
		copy := metadata
		path, found = candidate, &copy
	}
	return path, found, nil
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
		if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
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

func identityComponent(name, tableauID string) string {
	sum := sha256.Sum256([]byte(tableauID))
	suffix := "--" + hex.EncodeToString(sum[:])
	return portableComponent(name, "workbook", maxPortableComponentBytes-len(suffix)) + suffix
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
	return fmt.Sprintf("# %s\n\n- Kind: workbook\n- Tableau LUID: `%s`\n- Source environment: `%s`\n- Source site: `%s`\n- Source project: `%s`\n- Pulled at: `%s`\n- Canonical payload: `%s`\n", metadata.Name, metadata.TableauID, metadata.SourceEnvironment, metadata.SourceSite, metadata.SourceProjectName, metadata.PulledAt, metadata.CanonicalPayload)
}
