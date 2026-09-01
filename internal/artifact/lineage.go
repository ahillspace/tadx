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

const maxStandaloneLineageMetadataBytes = 64 * 1024

// LineageMetadata records source provenance for a metadata-only artifact.
type LineageMetadata struct {
	Kind               string `json:"kind"`
	ResourceKind       string `json:"resource_kind"`
	Name               string `json:"name"`
	TableauID          string `json:"tableau_id"`
	MetadataID         string `json:"metadata_id,omitempty"`
	ProjectPath        string `json:"project_path,omitempty"`
	SourceServerOrigin string `json:"source_server_origin"`
	SourceSiteLUID     string `json:"source_site_luid"`
	SourceEnvironment  string `json:"source_environment"`
	SourceSite         string `json:"source_site,omitempty"`
	PulledAt           string `json:"pulled_at"`
	Direction          string `json:"direction"`
	Depth              int    `json:"depth"`
	Complete           bool   `json:"complete"`
	NodeCount          *int   `json:"node_count,omitempty"`
	EdgeCount          *int   `json:"edge_count,omitempty"`
	LineagePath        string `json:"lineage_path"`
	Fingerprint        string `json:"fingerprint"`
}

// LineagePull describes one metadata-only lineage snapshot.
type LineagePull struct {
	Workspace string
	Metadata  LineageMetadata
	Lineage   LineageDocument
	// CountsKnown distinguishes an observed empty graph from unavailable counts.
	CountsKnown bool
	Overwrite   bool
}

// LineagePullResult identifies one materialized metadata-only artifact.
type LineagePullResult struct {
	Path        string
	LineagePath string
	Fingerprint string
}

// LineageManager owns metadata-only lineage persistence.
type LineageManager struct {
	now        func() time.Time
	operations directoryOperations
}

// NewLineageManager creates a metadata-only lineage manager.
func NewLineageManager(now func() time.Time) *LineageManager {
	if now == nil {
		now = time.Now
	}
	return &LineageManager{now: now, operations: defaultDirectoryOperations()}
}

// Pull creates or replaces one identity-bound metadata-only lineage artifact.
func (m *LineageManager) Pull(ctx context.Context, input LineagePull) (LineagePullResult, error) {
	if err := ctx.Err(); err != nil {
		return LineagePullResult{}, err
	}
	if m == nil {
		return LineagePullResult{}, errors.New("lineage artifact manager is not configured")
	}
	if err := validateLineagePull(input); err != nil {
		return LineagePullResult{}, err
	}
	origin, err := NormalizeServerOrigin(input.Metadata.SourceServerOrigin)
	if err != nil {
		return LineagePullResult{}, err
	}
	input.Metadata.SourceServerOrigin = origin
	input.Metadata.SourceSiteLUID = strings.TrimSpace(input.Metadata.SourceSiteLUID)
	workspace, err := filepath.Abs(input.Workspace)
	if err != nil {
		return LineagePullResult{}, err
	}
	if _, err := os.Stat(filepath.Join(workspace, "tadx.yaml")); err != nil {
		return LineagePullResult{}, fmt.Errorf("workspace %q does not contain tadx.yaml", workspace)
	}
	handle, err := lockWorkspace(workspace)
	if err != nil {
		return LineagePullResult{}, err
	}
	defer func() { _ = handle.Release() }()
	root, err := ensureStandaloneLineageRoot(workspace, input.Metadata.ResourceKind)
	if err != nil {
		return LineagePullResult{}, err
	}
	target := filepath.Join(root, lineageIdentityComponent(input.Metadata.Name, origin, input.Metadata.SourceSiteLUID, input.Metadata.ResourceKind, input.Metadata.TableauID))
	if err := protectExistingStandaloneLineage(ctx, target, input, origin); err != nil {
		return LineagePullResult{}, err
	}

	input.Lineage = normalizedLineage(input.Lineage)
	lineageData, err := json.MarshalIndent(input.Lineage, "", "  ")
	if err != nil {
		return LineagePullResult{}, fmt.Errorf("encode lineage graph: %w", err)
	}
	lineageData = append(lineageData, '\n')
	if len(lineageData) > maxLineageBytes {
		return LineagePullResult{}, errors.New("lineage sidecar exceeds its byte limit")
	}
	fingerprintValue := fingerprint(lineageData)
	metadata := input.Metadata
	metadata.Kind = "lineage"
	metadata.PulledAt = m.now().UTC().Format(time.RFC3339Nano)
	metadata.Direction = input.Lineage.Direction
	metadata.Depth = input.Lineage.Depth
	metadata.Complete = input.Lineage.Complete
	if input.CountsKnown || input.Lineage.Complete || len(input.Lineage.Nodes) > 0 || len(input.Lineage.Edges) > 0 {
		nodeCount, edgeCount := len(input.Lineage.Nodes), len(input.Lineage.Edges)
		metadata.NodeCount = &nodeCount
		metadata.EdgeCount = &edgeCount
	} else {
		metadata.NodeCount = nil
		metadata.EdgeCount = nil
	}
	metadata.LineagePath = "lineage.json"
	metadata.Fingerprint = fingerprintValue
	metadataData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return LineagePullResult{}, fmt.Errorf("encode lineage metadata: %w", err)
	}
	metadataData = append(metadataData, '\n')
	if len(metadataData) > maxStandaloneLineageMetadataBytes {
		return LineagePullResult{}, errors.New("lineage artifact metadata exceeds its byte limit")
	}

	staging, err := os.MkdirTemp(root, ".tadx-lineage-stage-")
	if err != nil {
		return LineagePullResult{}, err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	files := map[string][]byte{
		"metadata.json": metadataData,
		"lineage.json":  lineageData,
		"view.md":       []byte(standaloneLineageView(metadata)),
	}
	if err := writeStagedArtifact(staging, files, m.operations); err != nil {
		return LineagePullResult{}, err
	}
	if _, err := replaceDirectoryWithOperations(staging, target, m.operations); err != nil {
		return LineagePullResult{}, err
	}
	relative, err := filepath.Rel(workspace, target)
	if err != nil {
		return LineagePullResult{}, err
	}
	path := filepath.ToSlash(relative)
	return LineagePullResult{Path: path, LineagePath: filepath.ToSlash(filepath.Join(relative, "lineage.json")), Fingerprint: fingerprintValue}, nil
}

// isLineageResourceKind reports whether kind is a supported standalone lineage
// resource kind. It is the single source of truth for both persistence and the
// artifacts/lineage/<resourceKind> layout recognized during inventory.
func isLineageResourceKind(kind string) bool {
	return kind == "workbook" || kind == "published_datasource" || kind == "flow"
}

// normalizedLineage returns a copy whose Nodes and Edges are non-nil so an
// empty graph serializes as JSON [] rather than null.
func normalizedLineage(document LineageDocument) LineageDocument {
	if document.Nodes == nil {
		document.Nodes = []LineageNode{}
	}
	if document.Edges == nil {
		document.Edges = []LineageEdge{}
	}
	return document
}

func validateLineagePull(input LineagePull) error {
	if strings.TrimSpace(input.Workspace) == "" || strings.TrimSpace(input.Metadata.Name) == "" || strings.TrimSpace(input.Metadata.TableauID) == "" {
		return errors.New("lineage artifact requires workspace, name, and authoritative Tableau LUID")
	}
	if !isLineageResourceKind(input.Metadata.ResourceKind) {
		return fmt.Errorf("unsupported lineage resource kind %q", input.Metadata.ResourceKind)
	}
	if strings.TrimSpace(input.Metadata.SourceEnvironment) == "" || strings.TrimSpace(input.Metadata.SourceSiteLUID) == "" {
		return errors.New("lineage artifact requires source environment and site LUID")
	}
	if input.Lineage.Complete && strings.TrimSpace(input.Metadata.MetadataID) == "" {
		return errors.New("complete lineage artifact requires the root Metadata ID")
	}
	return validateLineage(input.Lineage)
}

func ensureStandaloneLineageRoot(workspace, kind string) (string, error) {
	base := filepath.Join(workspace, "artifacts", "lineage")
	for _, path := range []string{base, filepath.Join(base, kind)} {
		if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("lineage artifact root must not be a symbolic link")
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if err := os.MkdirAll(path, 0o700); err != nil {
			return "", err
		}
	}
	return filepath.Join(base, kind), nil
}

func protectExistingStandaloneLineage(ctx context.Context, target string, input LineagePull, origin string) error {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("lineage artifact target must be a directory and not a symbolic link")
	}
	metadata, err := readStandaloneLineageMetadata(target)
	if err != nil {
		return err
	}
	if metadata.SourceServerOrigin != origin || metadata.SourceSiteLUID != input.Metadata.SourceSiteLUID || metadata.ResourceKind != input.Metadata.ResourceKind || metadata.TableauID != input.Metadata.TableauID {
		return errors.New("existing lineage artifact has a different source identity")
	}
	current, err := fingerprintFile(ctx, filepath.Join(target, metadata.LineagePath))
	if err != nil {
		return err
	}
	if current != metadata.Fingerprint && !input.Overwrite {
		return errors.New("lineage artifact is dirty; use --overwrite to replace local edits")
	}
	return nil
}

func readStandaloneLineageMetadata(directory string) (LineageMetadata, error) {
	data, err := readBoundedFile(filepath.Join(directory, "metadata.json"), maxStandaloneLineageMetadataBytes)
	if err != nil {
		return LineageMetadata{}, err
	}
	var metadata LineageMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return LineageMetadata{}, err
	}
	if metadata.Kind != "lineage" || metadata.LineagePath != "lineage.json" || metadata.Fingerprint == "" {
		return LineageMetadata{}, errors.New("lineage artifact metadata is incomplete")
	}
	return metadata, nil
}

func lineageIdentityComponent(name, origin, site, kind, luid string) string {
	sum := sha256.Sum256([]byte(origin + "\x00" + site + "\x00" + kind + "\x00" + luid))
	suffix := "--" + hex.EncodeToString(sum[:8])
	return portableComponent(name, "lineage", maxPortableComponentBytes-len(suffix)) + suffix
}

func standaloneLineageView(metadata LineageMetadata) string {
	return fmt.Sprintf("# %s lineage\n\n- Resource kind: `%s`\n- Tableau LUID: `%s`\n- Metadata ID: `%s`\n- Source environment: `%s`\n- Source project: `%s`\n- Direction: `%s`\n- Depth: `%d`\n- Complete: `%t`\n- Nodes: `%s`\n- Edges: `%s`\n", metadata.Name, metadata.ResourceKind, metadata.TableauID, metadata.MetadataID, metadata.SourceEnvironment, metadata.ProjectPath, metadata.Direction, metadata.Depth, metadata.Complete, optionalCount(metadata.NodeCount), optionalCount(metadata.EdgeCount))
}

func optionalCount(value *int) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprintf("%d", *value)
}
