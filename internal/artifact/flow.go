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
	// MaxLineageNodes bounds one persisted lineage graph.
	MaxLineageNodes = 500
	// MaxLineageEdges bounds one persisted lineage graph.
	MaxLineageEdges      = 1000
	maxFlowMetadataBytes = 64 * 1024
	maxLineageBytes      = 4 * 1024 * 1024
)

// LineageNode preserves distinct Metadata and REST identities.
type LineageNode struct {
	MetadataID string `json:"metadata_id"`
	Kind       string `json:"kind"`
	RESTLUID   string `json:"rest_luid,omitempty"`
	Name       string `json:"name,omitempty"`
}

// LineageEdge is one factual directed relationship.
type LineageEdge struct {
	FromMetadataID string `json:"from_metadata_id"`
	ToMetadataID   string `json:"to_metadata_id"`
	Relationship   string `json:"relationship"`
}

// LineageFailure records bounded, sanitized provider failure context.
type LineageFailure struct {
	Provider     string `json:"provider"`
	Relation     string `json:"relation,omitempty"`
	RootKind     string `json:"root_kind"`
	RootRESTLUID string `json:"root_rest_luid"`
	RequestID    string `json:"request_id,omitempty"`
}

// LineageDocument is one bounded factual graph.
type LineageDocument struct {
	Complete  bool            `json:"complete"`
	Direction string          `json:"direction"`
	Depth     int             `json:"depth"`
	Failure   *LineageFailure `json:"failure,omitempty"`
	Nodes     []LineageNode   `json:"nodes"`
	Edges     []LineageEdge   `json:"edges"`
	Warnings  []string        `json:"warnings,omitempty"`
}

// FlowMetadata is persisted source provenance for one native flow.
type FlowMetadata struct {
	Kind                     string `json:"kind"`
	Name                     string `json:"name"`
	TableauID                string `json:"tableau_id"`
	SourceServerOrigin       string `json:"source_server_origin"`
	SourceSiteLUID           string `json:"source_site_luid"`
	SourceEnvironment        string `json:"source_environment"`
	SourceSite               string `json:"source_site"`
	SourceProjectName        string `json:"source_project_name"`
	SourceProjectID          string `json:"source_project_id"`
	FileType                 string `json:"file_type"`
	PulledAt                 string `json:"pulled_at"`
	CanonicalPayload         string `json:"canonical_payload"`
	LineageSidecar           string `json:"lineage_sidecar"`
	LineageComplete          bool   `json:"lineage_complete"`
	LocalBaselineFingerprint string `json:"local_baseline_fingerprint"`
}

// FlowPull describes one complete native flow artifact replacement.
type FlowPull struct {
	Workspace string
	Filename  string
	Content   []byte
	Metadata  FlowMetadata
	Lineage   LineageDocument
	Overwrite bool
}

// FlowPullResult identifies one materialized flow artifact.
type FlowPullResult struct {
	ArtifactPath          string
	CanonicalPath         string
	WorkspaceRelativePath string
	LineagePath           string
	BaselineFingerprint   string
	Warnings              []string
}

// FlowArtifact is one validated local native flow.
type FlowArtifact struct {
	Path, PayloadPath, Filename, Name, TableauID, Fingerprint string
	Size                                                      int64
	Metadata                                                  FlowMetadata
	Lineage                                                   LineageDocument
}

// FlowManager owns native flow artifact persistence.
type FlowManager struct {
	now        func() time.Time
	operations directoryOperations
}

// NewFlowManager creates a flow artifact manager.
func NewFlowManager(now func() time.Time) *FlowManager {
	if now == nil {
		now = time.Now
	}
	return &FlowManager{now: now, operations: defaultDirectoryOperations()}
}

// Pull creates or replaces one identity-bound flow artifact.
func (m *FlowManager) Pull(ctx context.Context, input FlowPull) (FlowPullResult, error) {
	if err := ctx.Err(); err != nil {
		return FlowPullResult{}, err
	}
	if m == nil {
		return FlowPullResult{}, errors.New("flow artifact manager is not configured")
	}
	if strings.TrimSpace(input.Workspace) == "" || strings.TrimSpace(input.Metadata.Name) == "" || strings.TrimSpace(input.Metadata.TableauID) == "" {
		return FlowPullResult{}, errors.New("flow artifact requires workspace, name, and Tableau ID")
	}
	origin, err := NormalizeServerOrigin(input.Metadata.SourceServerOrigin)
	if err != nil {
		return FlowPullResult{}, err
	}
	input.Metadata.SourceServerOrigin = origin
	input.Metadata.SourceSiteLUID = strings.TrimSpace(input.Metadata.SourceSiteLUID)
	if input.Metadata.SourceSiteLUID == "" || input.Metadata.SourceProjectID == "" || input.Metadata.SourceProjectName == "" {
		return FlowPullResult{}, errors.New("flow artifact requires source site and project identity")
	}
	extension := strings.ToLower(filepath.Ext(filepath.Base(input.Filename)))
	if extension != ".tfl" && extension != ".tflx" {
		return FlowPullResult{}, fmt.Errorf("unsupported flow artifact filename %q", input.Filename)
	}
	fileType := strings.TrimPrefix(extension, ".")
	if input.Metadata.FileType != "" && strings.ToLower(input.Metadata.FileType) != fileType {
		return FlowPullResult{}, errors.New("flow file type does not match the native filename")
	}
	if err := validateLineage(input.Lineage); err != nil {
		return FlowPullResult{}, err
	}
	workspace, err := filepath.Abs(input.Workspace)
	if err != nil {
		return FlowPullResult{}, err
	}
	if _, err := os.Stat(filepath.Join(workspace, "tadx.yaml")); err != nil {
		return FlowPullResult{}, fmt.Errorf("workspace %q does not contain tadx.yaml", workspace)
	}
	handle, err := lockWorkspace(workspace)
	if err != nil {
		return FlowPullResult{}, err
	}
	defer func() { _ = handle.Release() }()
	root, err := ensureFlowRoot(workspace)
	if err != nil {
		return FlowPullResult{}, err
	}
	target, existing, err := findFlow(root, origin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID)
	if err != nil {
		return FlowPullResult{}, err
	}
	if target == "" {
		target = filepath.Join(root, flowIdentityComponent(input.Metadata.Name, origin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID))
	}
	warnings := []string{}
	if existing != nil {
		canonical, err := canonicalFlowPath(target, existing.CanonicalPayload)
		if err != nil {
			return FlowPullResult{}, err
		}
		current, err := fingerprintFile(ctx, canonical)
		if err != nil {
			return FlowPullResult{}, err
		}
		if current != existing.LocalBaselineFingerprint && !input.Overwrite {
			return FlowPullResult{}, errors.New("flow artifact is dirty; use --overwrite to replace local edits")
		}
		if current != existing.LocalBaselineFingerprint {
			warnings = append(warnings, "Local flow edits were replaced because --overwrite was set.")
		}
	}
	base := strings.TrimSuffix(filepath.Base(input.Filename), filepath.Ext(input.Filename))
	filename := portableComponent(base, "flow", maxPortableComponentBytes-len(extension)) + extension
	input.Metadata.Kind = "flow"
	input.Metadata.FileType = fileType
	input.Metadata.PulledAt = m.now().UTC().Format(time.RFC3339Nano)
	input.Metadata.CanonicalPayload = filename
	input.Metadata.LineageSidecar = "lineage.json"
	input.Metadata.LineageComplete = input.Lineage.Complete
	input.Metadata.LocalBaselineFingerprint = fingerprint(input.Content)
	input.Lineage = normalizedLineage(input.Lineage)
	metadataData, err := json.MarshalIndent(input.Metadata, "", "  ")
	if err != nil {
		return FlowPullResult{}, err
	}
	metadataData = append(metadataData, '\n')
	lineageData, err := json.MarshalIndent(input.Lineage, "", "  ")
	if err != nil {
		return FlowPullResult{}, err
	}
	lineageData = append(lineageData, '\n')
	if len(lineageData) > maxLineageBytes {
		return FlowPullResult{}, errors.New("flow lineage sidecar exceeds its byte limit")
	}
	staging, err := os.MkdirTemp(root, ".tadx-flow-stage-")
	if err != nil {
		return FlowPullResult{}, err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	files := map[string][]byte{filename: input.Content, "metadata.json": metadataData, "lineage.json": lineageData, "view.md": []byte(flowView(input.Metadata, input.Lineage))}
	if err := writeStagedArtifact(staging, files, m.operations); err != nil {
		return FlowPullResult{}, err
	}
	replaceWarnings, err := replaceDirectoryWithOperations(staging, target, m.operations)
	if err != nil {
		return FlowPullResult{}, err
	}
	warnings = append(warnings, replaceWarnings...)
	rel, err := filepath.Rel(workspace, target)
	if err != nil {
		return FlowPullResult{}, err
	}
	return FlowPullResult{ArtifactPath: target, CanonicalPath: filepath.Join(target, filename), WorkspaceRelativePath: filepath.ToSlash(rel), LineagePath: filepath.ToSlash(filepath.Join(rel, "lineage.json")), BaselineFingerprint: input.Metadata.LocalBaselineFingerprint, Warnings: warnings}, nil
}

// Read validates one local flow artifact.
func (m *FlowManager) Read(ctx context.Context, path string) (FlowArtifact, error) {
	if err := ctx.Err(); err != nil {
		return FlowArtifact{}, err
	}
	metadata, err := readFlowMetadata(path)
	if err != nil {
		return FlowArtifact{}, err
	}
	canonical, err := canonicalFlowPath(path, metadata.CanonicalPayload)
	if err != nil {
		return FlowArtifact{}, err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return FlowArtifact{}, err
	}
	current, err := fingerprintFile(ctx, canonical)
	if err != nil {
		return FlowArtifact{}, err
	}
	lineageData, err := readBoundedFile(filepath.Join(path, metadata.LineageSidecar), maxLineageBytes)
	if err != nil {
		return FlowArtifact{}, err
	}
	var lineage LineageDocument
	if err := json.Unmarshal(lineageData, &lineage); err != nil {
		return FlowArtifact{}, err
	}
	if err := validateLineage(lineage); err != nil {
		return FlowArtifact{}, err
	}
	return FlowArtifact{Path: path, PayloadPath: canonical, Filename: metadata.CanonicalPayload, Name: metadata.Name, TableauID: metadata.TableauID, Fingerprint: current, Size: info.Size(), Metadata: metadata, Lineage: lineage}, nil
}

func validateLineage(value LineageDocument) error {
	if value.Depth < 1 || value.Depth > 3 {
		return errors.New("lineage depth must be between 1 and 3")
	}
	if value.Direction != "upstream" && value.Direction != "downstream" && value.Direction != "both" {
		return errors.New("lineage direction must be upstream, downstream, or both")
	}
	if len(value.Nodes) > MaxLineageNodes || len(value.Edges) > MaxLineageEdges {
		return errors.New("lineage graph exceeds the 500-node or 1000-edge bound")
	}
	seen := make(map[string]bool, len(value.Nodes))
	for _, node := range value.Nodes {
		if node.MetadataID == "" || node.Kind == "" || seen[node.MetadataID] {
			return errors.New("lineage nodes require unique Metadata IDs and kinds")
		}
		seen[node.MetadataID] = true
	}
	for _, edge := range value.Edges {
		if edge.FromMetadataID == "" || edge.ToMetadataID == "" || edge.Relationship == "" || !seen[edge.FromMetadataID] || !seen[edge.ToMetadataID] {
			return errors.New("lineage edge references an unknown node or omits its relationship")
		}
	}
	return nil
}

func ensureFlowRoot(workspace string) (string, error) {
	root := filepath.Join(workspace, "artifacts", "flow")
	if info, err := os.Lstat(root); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("flow artifact root must not be a symbolic link")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	return root, nil
}
func findFlow(root, origin, site, luid string) (string, *FlowMetadata, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(entry.Name(), ".tadx-") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		metadata, err := readFlowMetadata(path)
		if err != nil {
			return "", nil, err
		}
		if metadata.SourceServerOrigin == origin && metadata.SourceSiteLUID == site && metadata.TableauID == luid {
			return path, &metadata, nil
		}
	}
	return "", nil, nil
}
func readFlowMetadata(path string) (FlowMetadata, error) {
	data, err := readBoundedFile(filepath.Join(path, "metadata.json"), maxFlowMetadataBytes)
	if err != nil {
		return FlowMetadata{}, err
	}
	var value FlowMetadata
	if err := json.Unmarshal(data, &value); err != nil {
		return FlowMetadata{}, err
	}
	if err := validateFlowMetadata(value); err != nil {
		return FlowMetadata{}, err
	}
	return value, nil
}

func validateFlowMetadata(value FlowMetadata) error {
	required := []struct{ name, value string }{
		{"name", value.Name}, {"tableau_id", value.TableauID}, {"source_server_origin", value.SourceServerOrigin},
		{"source_site_luid", value.SourceSiteLUID}, {"source_environment", value.SourceEnvironment},
		{"source_project_name", value.SourceProjectName}, {"source_project_id", value.SourceProjectID},
		{"file_type", value.FileType}, {"pulled_at", value.PulledAt}, {"canonical_payload", value.CanonicalPayload},
		{"local_baseline_fingerprint", value.LocalBaselineFingerprint},
	}
	if value.Kind != "flow" || value.LineageSidecar != "lineage.json" {
		return errors.New("flow metadata is incomplete")
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("flow metadata requires %s", field.name)
		}
	}
	origin, err := NormalizeServerOrigin(value.SourceServerOrigin)
	if err != nil || origin != value.SourceServerOrigin {
		return errors.New("flow metadata requires a canonical source_server_origin")
	}
	if _, err := time.Parse(time.RFC3339Nano, value.PulledAt); err != nil {
		return fmt.Errorf("flow metadata has invalid pulled_at: %w", err)
	}
	digest := strings.TrimPrefix(value.LocalBaselineFingerprint, "sha256:")
	decoded, err := hex.DecodeString(digest)
	if !strings.HasPrefix(value.LocalBaselineFingerprint, "sha256:") || err != nil || len(decoded) != sha256.Size {
		return errors.New("flow metadata has invalid local_baseline_fingerprint")
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(value.CanonicalPayload)), ".")
	if extension != strings.ToLower(value.FileType) || (extension != "tfl" && extension != "tflx") {
		return errors.New("flow metadata file_type does not match the canonical payload")
	}
	return nil
}
func canonicalFlowPath(directory, payload string) (string, error) {
	if payload == "" || filepath.IsAbs(payload) || filepath.Base(payload) != payload || filepath.Clean(payload) != payload {
		return "", fmt.Errorf("invalid flow canonical payload %q", payload)
	}
	extension := strings.ToLower(filepath.Ext(payload))
	if extension != ".tfl" && extension != ".tflx" {
		return "", fmt.Errorf("unsupported flow canonical payload %q", payload)
	}
	canonical := filepath.Join(directory, payload)
	info, err := os.Lstat(canonical)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("flow canonical payload must be a regular file and not a symbolic link")
	}
	return canonical, nil
}
func readBoundedFile(path string, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, errors.New("file byte limit must not be negative")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("file %q must be a regular file and not a symbolic link", filepath.Base(path))
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	currentPathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if currentPathInfo.Mode()&os.ModeSymlink != 0 || !currentPathInfo.Mode().IsRegular() ||
		!openedInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) || !os.SameFile(currentPathInfo, openedInfo) {
		return nil, fmt.Errorf("file %q changed while opening", filepath.Base(path))
	}
	data, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return nil, err
	}
	var extra [1]byte
	n, err := file.Read(extra[:])
	if n > 0 {
		return nil, fmt.Errorf("file %q exceeds its byte limit", filepath.Base(path))
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return data, nil
}
func flowIdentityComponent(name, origin, site, luid string) string {
	sum := sha256.Sum256([]byte(origin + "\x00" + site + "\x00" + luid))
	suffix := "--" + hex.EncodeToString(sum[:8])
	return portableComponent(name, "flow", maxPortableComponentBytes-len(suffix)) + suffix
}
func flowView(metadata FlowMetadata, lineage LineageDocument) string {
	return fmt.Sprintf("# %s\n\n- Kind: flow\n- Tableau LUID: `%s`\n- Source environment: `%s`\n- Source project: `%s`\n- Native package: `%s`\n- Lineage complete: `%t`\n", metadata.Name, metadata.TableauID, metadata.SourceEnvironment, metadata.SourceProjectName, metadata.CanonicalPayload, lineage.Complete)
}
