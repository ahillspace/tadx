package artifact

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PullPreview selects the same source identity and overwrite policy as acquisition.
type PullPreview struct {
	Workspace, Kind, ResourceKind, Name, LUID, ServerOrigin, SiteLUID string
	Overwrite                                                         bool
}

type PullPreviewResult struct {
	Path   string
	Exists bool
}

// PreviewPull checks existing managed artifacts without locks, recovery, or writes.
// Remote payload validation and write permissions remain execution prerequisites.
func PreviewPull(ctx context.Context, input PullPreview) (PullPreviewResult, error) {
	if err := ctx.Err(); err != nil {
		return PullPreviewResult{}, err
	}
	if input.Workspace == "" || input.Name == "" || input.LUID == "" || strings.TrimSpace(input.SiteLUID) == "" {
		return PullPreviewResult{}, errors.New("acquisition preview requires workspace and complete source identity")
	}
	origin, err := NormalizeServerOrigin(input.ServerOrigin)
	if err != nil {
		return PullPreviewResult{}, err
	}
	workspace, err := filepath.Abs(input.Workspace)
	if err != nil {
		return PullPreviewResult{}, err
	}
	if _, err := os.Stat(filepath.Join(workspace, "tadx.yaml")); err != nil {
		return PullPreviewResult{}, errors.New("acquisition preview requires an initialized workspace")
	}
	root := filepath.Join(workspace, "artifacts", input.Kind)
	component := ""
	switch input.Kind {
	case "workbook":
		component = identityComponent(input.Name, origin, input.SiteLUID, input.LUID)
	case "datasource":
		component = datasourceIdentityComponent(input.Name, origin, input.SiteLUID, input.LUID)
	case "flow":
		component = flowIdentityComponent(input.Name, origin, input.SiteLUID, input.LUID)
	case "pulse-definition":
		component = pulseDefinitionIdentityComponent(input.Name, origin, input.SiteLUID, input.LUID)
	case "lineage":
		if input.ResourceKind != "workbook" && input.ResourceKind != "published_datasource" && input.ResourceKind != "flow" {
			return PullPreviewResult{}, errors.New("unsupported lineage root kind")
		}
		root = filepath.Join(root, input.ResourceKind)
		component = lineageIdentityComponent(input.Name, origin, input.SiteLUID, input.ResourceKind, input.LUID)
	default:
		return PullPreviewResult{}, errors.New("unsupported acquisition artifact kind")
	}
	// Check each existing parent independently; never create missing roots.
	for candidate := root; candidate != workspace; candidate = filepath.Dir(candidate) {
		info, statErr := os.Lstat(candidate)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return PullPreviewResult{}, statErr
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return PullPreviewResult{}, errors.New("artifact root must be a directory and not a symbolic link")
		}
	}
	target := filepath.Join(root, component)
	exists := false
	canonical, baseline, bundleBaseline := "", "", ""
	if _, statErr := os.Stat(root); statErr == nil {
		switch input.Kind {
		case "workbook":
			var metadata *WorkbookMetadata
			var found string
			found, metadata, err = findBySourceIdentity(root, origin, input.SiteLUID, input.LUID)
			if err == nil && metadata != nil {
				target, exists = found, true
				canonical, err = canonicalWorkbookPath(target, metadata.CanonicalPayload)
				baseline = metadata.LocalBaselineFingerprint
			}
		case "datasource":
			var metadata *DatasourceMetadata
			var found string
			found, metadata, err = findDatasourceBySourceIdentity(root, origin, input.SiteLUID, input.LUID)
			if err == nil && metadata != nil {
				target, exists = found, true
				canonical, err = canonicalDatasourcePath(target, metadata.CanonicalPayload)
				baseline = metadata.LocalBaselineFingerprint
			}
		case "flow":
			var metadata *FlowMetadata
			var found string
			found, metadata, err = findFlow(root, origin, input.SiteLUID, input.LUID)
			if err == nil && metadata != nil {
				target, exists = found, true
				canonical, err = canonicalFlowPath(target, metadata.CanonicalPayload)
				baseline = metadata.LocalBaselineFingerprint
			}
		case "pulse-definition":
			var metadata *PulseDefinitionMetadata
			var found string
			found, metadata, err = findPulseDefinition(root, origin, input.SiteLUID, input.LUID)
			if err == nil && metadata != nil {
				target, exists = found, true
				canonical, err = canonicalPulseDefinitionPath(target, metadata.CanonicalPayload)
				baseline, bundleBaseline = metadata.LocalBaselineFingerprint, metadata.BundleFingerprint
			}
		case "lineage":
			err = protectExistingStandaloneLineage(ctx, target, LineagePull{Overwrite: input.Overwrite, Metadata: LineageMetadata{ResourceKind: input.ResourceKind, SourceSiteLUID: input.SiteLUID, TableauID: input.LUID}}, origin)
			if err == nil {
				_, statErr := os.Lstat(target)
				exists = statErr == nil
			}
		}
		if err != nil {
			return PullPreviewResult{}, err
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return PullPreviewResult{}, statErr
	}
	if !exists {
		if _, statErr := os.Lstat(target); statErr == nil {
			return PullPreviewResult{}, errors.New("prospective artifact path is occupied by an unmanaged or different source artifact")
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return PullPreviewResult{}, statErr
		}
	}
	if canonical != "" {
		current, err := fingerprintFile(ctx, canonical)
		if err != nil {
			return PullPreviewResult{}, err
		}
		if current != baseline && !input.Overwrite {
			return PullPreviewResult{}, fmt.Errorf("%s artifact is dirty; use --overwrite to replace local edits", input.Kind)
		}
	}
	if bundleBaseline != "" {
		bundle, err := readPulseBundleFile(filepath.Join(target, "bundle.json"))
		if err != nil {
			return PullPreviewResult{}, err
		}
		if fingerprint(bundle) != bundleBaseline && !input.Overwrite {
			return PullPreviewResult{}, errors.New("Pulse bundle is dirty; use --overwrite to replace local edits")
		}
	}
	relative, err := filepath.Rel(workspace, target)
	if err != nil || !pathContained(workspace, target) {
		return PullPreviewResult{}, errors.New("preview artifact path escapes workspace")
	}
	return PullPreviewResult{Path: filepath.ToSlash(relative), Exists: exists}, nil
}
