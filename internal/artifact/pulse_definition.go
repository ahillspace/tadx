package artifact

import (
	"bytes"
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
	maxPulseDefinitionMetadataBytes = 64 * 1024
	maxPulseDefinitionResourceBytes = 4 * 1024 * 1024
	pulseDefinitionResource         = "resource.json"
)

// PulseDefinitionMetadata is the persisted source provenance for a definition.
type PulseDefinitionMetadata struct {
	Kind                     string `json:"kind"`
	Name                     string `json:"name"`
	TableauID                string `json:"tableau_id"`
	DatasourceLUID           string `json:"datasource_luid"`
	SourceServerOrigin       string `json:"source_server_origin"`
	SourceSiteLUID           string `json:"source_site_luid"`
	SourceEnvironment        string `json:"source_environment"`
	SourceSite               string `json:"source_site"`
	PulledAt                 string `json:"pulled_at"`
	CanonicalPayload         string `json:"canonical_payload"`
	LocalBaselineFingerprint string `json:"local_baseline_fingerprint"`
	BundleFingerprint        string `json:"bundle_fingerprint,omitempty"`
}

// PulseDefinitionPull describes one complete definition artifact replacement.
type PulseDefinitionPull struct {
	Workspace     string
	Configuration []byte
	Bundle        []byte
	Metadata      PulseDefinitionMetadata
	Overwrite     bool
}

// PulseDefinitionPullResult identifies the materialized definition artifact.
type PulseDefinitionPullResult struct {
	ArtifactPath          string
	CanonicalPath         string
	WorkspaceRelativePath string
	BaselineFingerprint   string
	Warnings              []string
}

// PulseDefinitionArtifact is a validated local definition and its provenance.
type PulseDefinitionArtifact struct {
	Path          string
	PayloadPath   string
	Fingerprint   string
	Configuration []byte
	Metadata      PulseDefinitionMetadata
}

// PulseDefinitionManager owns managed Pulse definition persistence.
type PulseDefinitionManager struct {
	now        func() time.Time
	operations directoryOperations
}

// NewPulseDefinitionManager creates a definition artifact manager.
func NewPulseDefinitionManager(now func() time.Time) *PulseDefinitionManager {
	if now == nil {
		now = time.Now
	}
	return &PulseDefinitionManager{now: now, operations: defaultDirectoryOperations()}
}

// Pull creates or safely refreshes one identity-matched definition artifact.
func (m *PulseDefinitionManager) Pull(ctx context.Context, input PulseDefinitionPull) (PulseDefinitionPullResult, error) {
	if err := ctx.Err(); err != nil {
		return PulseDefinitionPullResult{}, err
	}
	if m == nil {
		return PulseDefinitionPullResult{}, errors.New("Pulse definition artifact manager is not configured")
	}
	if strings.TrimSpace(input.Workspace) == "" || strings.TrimSpace(input.Metadata.TableauID) == "" || strings.TrimSpace(input.Metadata.Name) == "" || strings.TrimSpace(input.Metadata.DatasourceLUID) == "" {
		return PulseDefinitionPullResult{}, errors.New("Pulse definition artifact requires workspace, Tableau ID, name, and datasource LUID")
	}
	origin, err := NormalizeServerOrigin(input.Metadata.SourceServerOrigin)
	if err != nil {
		return PulseDefinitionPullResult{}, err
	}
	input.Metadata.SourceServerOrigin = origin
	input.Metadata.SourceSiteLUID = strings.TrimSpace(input.Metadata.SourceSiteLUID)
	input.Metadata.SourceEnvironment = strings.TrimSpace(input.Metadata.SourceEnvironment)
	input.Metadata.SourceSite = strings.TrimSpace(input.Metadata.SourceSite)
	if input.Metadata.SourceSiteLUID == "" || input.Metadata.SourceEnvironment == "" || input.Metadata.SourceSite == "" {
		return PulseDefinitionPullResult{}, errors.New("Pulse definition artifact requires source site identity")
	}
	configuration, identity, err := canonicalPulseDefinition(input.Configuration)
	if err != nil {
		return PulseDefinitionPullResult{}, err
	}
	if identity.ID != input.Metadata.TableauID || identity.Name != input.Metadata.Name || identity.DatasourceID != input.Metadata.DatasourceLUID {
		return PulseDefinitionPullResult{}, errors.New("Pulse definition configuration identity does not match artifact metadata")
	}
	if len(input.Bundle) > 0 {
		bundle, err := DecodePulseBundle(input.Bundle)
		if err != nil {
			return PulseDefinitionPullResult{}, err
		}
		if bundle.DefinitionLUID != identity.ID || bundle.SourceServerOrigin != origin || bundle.SourceSiteLUID != input.Metadata.SourceSiteLUID {
			return PulseDefinitionPullResult{}, errors.New("Pulse bundle identity does not match definition provenance")
		}
		canonical, _, err := canonicalPulseDefinition(bundle.Definition)
		if err != nil || !bytes.Equal(canonical, configuration) {
			return PulseDefinitionPullResult{}, errors.New("Pulse bundle definition differs from its canonical resource")
		}
	}
	workspace, err := filepath.Abs(input.Workspace)
	if err != nil {
		return PulseDefinitionPullResult{}, err
	}
	if _, err := os.Stat(filepath.Join(workspace, "tadx.yaml")); err != nil {
		return PulseDefinitionPullResult{}, fmt.Errorf("workspace %q does not contain tadx.yaml", workspace)
	}
	handle, err := lockWorkspace(workspace)
	if err != nil {
		return PulseDefinitionPullResult{}, err
	}
	defer func() { _ = handle.Release() }()
	root, err := ensurePulseDefinitionRoot(workspace)
	if err != nil {
		return PulseDefinitionPullResult{}, err
	}
	target, existing, err := findPulseDefinition(root, origin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID)
	if err != nil {
		return PulseDefinitionPullResult{}, err
	}
	if target == "" {
		target = filepath.Join(root, pulseDefinitionIdentityComponent(input.Metadata.Name, origin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID))
	}
	warnings := []string{}
	if existing != nil {
		canonical, err := canonicalPulseDefinitionPath(target, existing.CanonicalPayload)
		if err != nil {
			return PulseDefinitionPullResult{}, err
		}
		current, err := fingerprintFile(ctx, canonical)
		if err != nil {
			return PulseDefinitionPullResult{}, err
		}
		if current != existing.LocalBaselineFingerprint && !input.Overwrite {
			return PulseDefinitionPullResult{}, errors.New("Pulse definition artifact is dirty; use --overwrite to replace local edits")
		}
		if existing.BundleFingerprint != "" {
			bundlePath := filepath.Join(target, "bundle.json")
			bundle, err := readPulseBundleFile(bundlePath)
			if err != nil {
				return PulseDefinitionPullResult{}, err
			}
			if fingerprint(bundle) != existing.BundleFingerprint && !input.Overwrite {
				return PulseDefinitionPullResult{}, errors.New("Pulse bundle is dirty; use --overwrite to replace local edits")
			}
			if fingerprint(bundle) != existing.BundleFingerprint {
				warnings = append(warnings, "Local Pulse bundle edits were replaced because --overwrite was set.")
			}
			if len(input.Bundle) == 0 {
				return PulseDefinitionPullResult{}, errors.New("a complete Pulse bundle cannot be replaced with a definition-only snapshot")
			}
		}
		if current != existing.LocalBaselineFingerprint {
			warnings = append(warnings, "Local Pulse definition edits were replaced because --overwrite was set.")
		}
	}
	baseline := fingerprint(configuration)
	metadata := input.Metadata
	metadata.Kind = "pulse-definition"
	metadata.PulledAt = m.now().UTC().Format(time.RFC3339Nano)
	metadata.CanonicalPayload = pulseDefinitionResource
	metadata.LocalBaselineFingerprint = baseline
	if len(input.Bundle) > 0 {
		metadata.BundleFingerprint = fingerprint(input.Bundle)
	}
	if err := validatePulseDefinitionMetadata(metadata); err != nil {
		return PulseDefinitionPullResult{}, err
	}
	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return PulseDefinitionPullResult{}, fmt.Errorf("encode Pulse definition metadata: %w", err)
	}
	metadataBytes = append(metadataBytes, '\n')
	staging, err := os.MkdirTemp(root, ".tadx-pulse-definition-stage-")
	if err != nil {
		return PulseDefinitionPullResult{}, fmt.Errorf("create Pulse definition staging directory: %w", err)
	}
	defer os.RemoveAll(staging)
	files := map[string][]byte{
		pulseDefinitionResource: configuration,
		"metadata.json":         metadataBytes,
		"view.md":               []byte(pulseDefinitionView(metadata)),
	}
	if len(input.Bundle) > 0 {
		files["bundle.json"] = input.Bundle
	}
	if err := writeStagedArtifact(staging, files, m.operations); err != nil {
		return PulseDefinitionPullResult{}, err
	}
	replaceWarnings, err := replaceDirectoryWithOperations(staging, target, m.operations)
	if err != nil {
		return PulseDefinitionPullResult{}, err
	}
	warnings = append(warnings, replaceWarnings...)
	relative, err := filepath.Rel(workspace, target)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return PulseDefinitionPullResult{}, fmt.Errorf("Pulse definition artifact path %q escapes workspace %q", target, workspace)
	}
	relative = filepath.ToSlash(relative)
	return PulseDefinitionPullResult{
		ArtifactPath:          target,
		CanonicalPath:         relative + "/" + pulseDefinitionResource,
		WorkspaceRelativePath: relative,
		BaselineFingerprint:   baseline,
		Warnings:              warnings,
	}, nil
}

// Read validates one local Pulse definition artifact.
func (m *PulseDefinitionManager) Read(ctx context.Context, path string) (PulseDefinitionArtifact, error) {
	if err := ctx.Err(); err != nil {
		return PulseDefinitionArtifact{}, err
	}
	metadata, err := readPulseDefinitionMetadata(path)
	if err != nil {
		return PulseDefinitionArtifact{}, err
	}
	payload, err := canonicalPulseDefinitionPath(path, metadata.CanonicalPayload)
	if err != nil {
		return PulseDefinitionArtifact{}, err
	}
	configuration, err := readBoundedFile(payload, maxPulseDefinitionResourceBytes)
	if err != nil {
		return PulseDefinitionArtifact{}, err
	}
	_, identity, err := canonicalPulseDefinition(configuration)
	if err != nil {
		return PulseDefinitionArtifact{}, err
	}
	if identity.ID != metadata.TableauID || identity.Name != metadata.Name || identity.DatasourceID != metadata.DatasourceLUID {
		return PulseDefinitionArtifact{}, errors.New("Pulse definition configuration identity does not match artifact metadata")
	}
	current, err := fingerprintFile(ctx, payload)
	if err != nil {
		return PulseDefinitionArtifact{}, err
	}
	return PulseDefinitionArtifact{Path: path, PayloadPath: payload, Fingerprint: current, Configuration: configuration, Metadata: metadata}, nil
}

type pulseDefinitionIdentity struct {
	ID           string
	Name         string
	DatasourceID string
}

func canonicalPulseDefinition(value []byte) ([]byte, pulseDefinitionIdentity, error) {
	if len(value) == 0 || len(value) > maxPulseDefinitionResourceBytes {
		return nil, pulseDefinitionIdentity{}, errors.New("Pulse definition configuration is empty or exceeds its byte limit")
	}
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, pulseDefinitionIdentity{}, fmt.Errorf("decode Pulse definition configuration: %w", err)
	}
	var trailing any
	if trailingErr := decoder.Decode(&trailing); document == nil || !errors.Is(trailingErr, io.EOF) {
		return nil, pulseDefinitionIdentity{}, errors.New("Pulse definition configuration must contain exactly one JSON object")
	}
	metadata, ok := document["metadata"].(map[string]any)
	if !ok {
		return nil, pulseDefinitionIdentity{}, errors.New("Pulse definition configuration requires metadata")
	}
	specification, ok := document["specification"].(map[string]any)
	if !ok {
		return nil, pulseDefinitionIdentity{}, errors.New("Pulse definition configuration requires specification")
	}
	datasource, ok := specification["datasource"].(map[string]any)
	if !ok {
		return nil, pulseDefinitionIdentity{}, errors.New("Pulse definition configuration requires specification.datasource")
	}
	identity := pulseDefinitionIdentity{ID: jsonString(metadata["id"]), Name: jsonString(metadata["name"]), DatasourceID: jsonString(datasource["id"])}
	if identity.ID == "" || identity.Name == "" || identity.DatasourceID == "" {
		return nil, pulseDefinitionIdentity{}, errors.New("Pulse definition configuration requires metadata id and name plus datasource id")
	}
	canonical, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, pulseDefinitionIdentity{}, fmt.Errorf("encode Pulse definition configuration: %w", err)
	}
	return append(canonical, '\n'), identity, nil
}

func jsonString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func ensurePulseDefinitionRoot(workspace string) (string, error) {
	root := filepath.Join(workspace, "artifacts", "pulse-definition")
	if info, err := os.Lstat(root); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("Pulse definition artifact root must not be a symbolic link")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	return root, nil
}

func findPulseDefinition(root, origin, site, luid string) (string, *PulseDefinitionMetadata, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(entry.Name(), ".tadx-") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		metadata, err := readPulseDefinitionMetadata(path)
		if err != nil {
			return "", nil, err
		}
		if metadata.SourceServerOrigin == origin && metadata.SourceSiteLUID == site && metadata.TableauID == luid {
			return path, &metadata, nil
		}
	}
	return "", nil, nil
}

func readPulseDefinitionMetadata(path string) (PulseDefinitionMetadata, error) {
	data, err := readBoundedFile(filepath.Join(path, "metadata.json"), maxPulseDefinitionMetadataBytes)
	if err != nil {
		return PulseDefinitionMetadata{}, err
	}
	var value PulseDefinitionMetadata
	if err := json.Unmarshal(data, &value); err != nil {
		return PulseDefinitionMetadata{}, err
	}
	if err := validatePulseDefinitionMetadata(value); err != nil {
		return PulseDefinitionMetadata{}, err
	}
	return value, nil
}

func validatePulseDefinitionMetadata(value PulseDefinitionMetadata) error {
	if value.Kind != "pulse-definition" || value.CanonicalPayload != pulseDefinitionResource {
		return errors.New("Pulse definition metadata is incomplete")
	}
	required := []struct{ name, value string }{
		{"name", value.Name}, {"tableau_id", value.TableauID}, {"datasource_luid", value.DatasourceLUID},
		{"source_server_origin", value.SourceServerOrigin}, {"source_site_luid", value.SourceSiteLUID},
		{"source_environment", value.SourceEnvironment}, {"source_site", value.SourceSite},
		{"pulled_at", value.PulledAt}, {"local_baseline_fingerprint", value.LocalBaselineFingerprint},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("Pulse definition metadata requires %s", field.name)
		}
	}
	origin, err := NormalizeServerOrigin(value.SourceServerOrigin)
	if err != nil || origin != value.SourceServerOrigin {
		return errors.New("Pulse definition metadata requires a canonical source_server_origin")
	}
	if _, err := time.Parse(time.RFC3339Nano, value.PulledAt); err != nil {
		return fmt.Errorf("Pulse definition metadata has invalid pulled_at: %w", err)
	}
	digest := strings.TrimPrefix(value.LocalBaselineFingerprint, "sha256:")
	decoded, err := hex.DecodeString(digest)
	if !strings.HasPrefix(value.LocalBaselineFingerprint, "sha256:") || err != nil || len(decoded) != sha256.Size {
		return errors.New("Pulse definition metadata has invalid local_baseline_fingerprint")
	}
	return nil
}

func canonicalPulseDefinitionPath(directory, payload string) (string, error) {
	if payload != pulseDefinitionResource {
		return "", fmt.Errorf("invalid Pulse definition canonical payload %q", payload)
	}
	canonical := filepath.Join(directory, payload)
	info, err := os.Lstat(canonical)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("Pulse definition canonical payload must be a regular file and not a symbolic link")
	}
	return canonical, nil
}

func pulseDefinitionIdentityComponent(name, origin, site, luid string) string {
	sum := sha256.Sum256([]byte(origin + "\x00" + site + "\x00" + luid))
	suffix := "--" + hex.EncodeToString(sum[:8])
	return portableComponent(name, "pulse-definition", maxPortableComponentBytes-len(suffix)) + suffix
}

func pulseDefinitionView(metadata PulseDefinitionMetadata) string {
	return fmt.Sprintf("# %s\n\n- Kind: pulse-definition\n- Tableau LUID: `%s`\n- Datasource LUID: `%s`\n- Source environment: `%s`\n- Canonical resource: `%s`\n", metadata.Name, metadata.TableauID, metadata.DatasourceLUID, metadata.SourceEnvironment, metadata.CanonicalPayload)
}
