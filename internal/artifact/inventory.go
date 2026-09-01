package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	// StateClean means the canonical payload matches its pulled baseline.
	StateClean = "clean"
	// StateDirty means the canonical payload differs from its pulled baseline.
	StateDirty = "dirty"
	// StateMissing means the managed canonical payload is missing.
	StateMissing = "missing"
	// StateInvalid means metadata or containment validation failed.
	StateInvalid = "invalid"

	maxInventoryLimit = 1000
	maxInventoryScan  = 10000
)

// Selector identifies one exact managed artifact.
type Selector struct {
	Kind         string
	LUID         string
	ServerOrigin string
	SiteLUID     string
	Path         string
}

// Item is one bounded, validated managed-artifact record.
type Item struct {
	Kind                string
	LUID                string
	Name                string
	Path                string
	CanonicalPath       string
	State               string
	ServerOrigin        string
	SiteLUID            string
	BaselineFingerprint string
	CurrentFingerprint  string
	TreeFingerprint     string
	Warnings            []string
}

// InventoryOptions bounds one artifact inventory page.
type InventoryOptions struct {
	Limit  int
	Cursor string
}

// InventoryPage is one deterministic bounded page.
type InventoryPage struct {
	Items        []Item
	Returned     int
	Total        int
	Limit        int
	NextCursor   string
	ScanComplete bool
	Clean        int
	Dirty        int
	Missing      int
	Invalid      int
	Warnings     []string
}

// Inventory enumerates only known managed artifact roots.
func Inventory(ctx context.Context, workspace string, options InventoryOptions) (InventoryPage, error) {
	return inventoryWithScanLimit(ctx, workspace, options, maxInventoryScan)
}

func inventoryWithScanLimit(ctx context.Context, workspace string, options InventoryOptions, scanLimit int) (InventoryPage, error) {
	if err := ctx.Err(); err != nil {
		return InventoryPage{}, err
	}
	if options.Limit <= 0 || options.Limit > maxInventoryLimit {
		return InventoryPage{}, fmt.Errorf("artifact inventory limit must be between 1 and %d", maxInventoryLimit)
	}
	offset := 0
	if options.Cursor != "" {
		value, err := strconv.Atoi(options.Cursor)
		if err != nil || value < 0 {
			return InventoryPage{}, errors.New("artifact inventory cursor is invalid")
		}
		offset = value
	}
	root, err := validateWorkspaceRoot(workspace)
	if err != nil {
		return InventoryPage{}, err
	}
	items, complete, err := scanManagedArtifacts(ctx, root, scanLimit)
	if err != nil {
		return InventoryPage{}, err
	}
	if !complete {
		return InventoryPage{}, errors.New("artifact inventory exceeds its bounded scan limit")
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if items[i].Path != items[j].Path {
			return items[i].Path < items[j].Path
		}
		return items[i].LUID < items[j].LUID
	})
	page := InventoryPage{Limit: options.Limit, ScanComplete: complete}
	if complete {
		page.Total = len(items)
	}
	if offset >= len(items) {
		return page, nil
	}
	end := min(offset+options.Limit, len(items))
	page.Items = items[offset:end]
	page.Returned = len(page.Items)
	for _, item := range page.Items {
		switch item.State {
		case StateClean:
			page.Clean++
		case StateDirty:
			page.Dirty++
		case StateMissing:
			page.Missing++
		case StateInvalid:
			page.Invalid++
		}
		for _, warning := range item.Warnings {
			page.Warnings = append(page.Warnings, item.Path+": "+warning)
		}
	}
	if end < len(items) || !complete {
		page.NextCursor = strconv.Itoa(end)
	}
	return page, nil
}

// Resolve returns one exact valid managed artifact.
func Resolve(ctx context.Context, workspace string, selector Selector) (Item, error) {
	root, err := validateWorkspaceRoot(workspace)
	if err != nil {
		return Item{}, err
	}
	if selector.Path != "" {
		return resolveRelativePath(ctx, root, selector)
	}
	if selector.Kind == "" || selector.LUID == "" {
		return Item{}, errors.New("artifact selector requires kind and LUID, or an exact managed relative path")
	}
	items, _, err := scanManagedArtifacts(ctx, root, maxInventoryScan)
	if err != nil {
		return Item{}, err
	}
	matches := make([]Item, 0, 1)
	for _, item := range items {
		if item.State == StateInvalid || item.Kind != selector.Kind || item.LUID != selector.LUID {
			continue
		}
		if selector.ServerOrigin != "" && item.ServerOrigin != selector.ServerOrigin {
			continue
		}
		if selector.SiteLUID != "" && item.SiteLUID != selector.SiteLUID {
			continue
		}
		matches = append(matches, item)
	}
	if len(matches) == 0 {
		return Item{}, errors.New("no managed artifact matches the exact selector")
	}
	if len(matches) > 1 {
		return Item{}, errors.New("managed artifact selector is ambiguous across source identities")
	}
	return matches[0], nil
}

func scanManagedArtifacts(ctx context.Context, workspace string, scanLimit int) ([]Item, bool, error) {
	var items []Item
	for _, kind := range []string{"datasource", "flow", "workbook"} {
		kindRoot := filepath.Join(workspace, "artifacts", kind)
		entries, err := os.ReadDir(kindRoot)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
			if strings.HasPrefix(entry.Name(), ".tadx-") {
				continue
			}
			if len(items) >= scanLimit {
				return items, false, nil
			}
			relative := filepath.ToSlash(filepath.Join("artifacts", kind, entry.Name()))
			if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
				items = append(items, Item{Kind: kind, Path: relative, State: StateInvalid, Warnings: []string{"Managed artifact entry is not a regular directory."}})
				continue
			}
			item, itemErr := inspectArtifact(ctx, workspace, kind, filepath.Join(kindRoot, entry.Name()))
			if itemErr != nil {
				items = append(items, Item{Kind: kind, Path: relative, State: StateInvalid, Warnings: []string{invalidArtifactWarning(itemErr)}})
				continue
			}
			items = append(items, item)
		}
	}
	return items, true, nil
}

func resolveRelativePath(ctx context.Context, workspace string, selector Selector) (Item, error) {
	if filepath.IsAbs(selector.Path) || strings.Contains(selector.Path, `\`) {
		return Item{}, errors.New("artifact path must be workspace-relative and slash-delimited")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(selector.Path)))
	if clean != selector.Path || clean == "." || strings.HasPrefix(clean, "../") {
		return Item{}, errors.New("artifact path escapes the workspace")
	}
	parts := strings.Split(clean, "/")
	if len(parts) != 3 || parts[0] != "artifacts" || (parts[1] != "workbook" && parts[1] != "datasource" && parts[1] != "flow") {
		return Item{}, errors.New("artifact path is outside a managed artifact root")
	}
	if selector.Kind != "" && selector.Kind != parts[1] {
		return Item{}, errors.New("artifact path kind does not match selector kind")
	}
	item, err := inspectArtifact(ctx, workspace, parts[1], filepath.Join(workspace, filepath.FromSlash(clean)))
	if err != nil {
		return Item{}, err
	}
	if selector.LUID != "" && selector.LUID != item.LUID {
		return Item{}, errors.New("artifact path identity does not match selector LUID")
	}
	return item, nil
}

func inspectArtifact(ctx context.Context, workspace, kind, directory string) (Item, error) {
	if err := validateArtifactDirectory(workspace, kind, directory); err != nil {
		return Item{}, err
	}
	item := Item{Kind: kind}
	var canonical string
	switch kind {
	case "workbook":
		metadata, err := readMetadata(directory)
		if err != nil {
			return Item{}, err
		}
		if err := validateWorkbookMetadata(metadata); err != nil {
			return Item{}, err
		}
		item.LUID, item.Name = metadata.TableauID, metadata.Name
		item.ServerOrigin, item.SiteLUID = metadata.SourceServerOrigin, metadata.SourceSiteLUID
		item.BaselineFingerprint = metadata.LocalBaselineFingerprint
		canonical, err = inventoryCanonicalPath(directory, metadata.CanonicalPayload, ".twb", ".twbx")
		if err != nil {
			return Item{}, err
		}
	case "datasource":
		metadata, err := readDatasourceMetadata(directory)
		if err != nil {
			return Item{}, err
		}
		if err := validateDatasourceMetadata(metadata); err != nil {
			return Item{}, err
		}
		item.LUID, item.Name = metadata.TableauID, metadata.Name
		item.ServerOrigin, item.SiteLUID = metadata.SourceServerOrigin, metadata.SourceSiteLUID
		item.BaselineFingerprint = metadata.LocalBaselineFingerprint
		canonical, err = inventoryCanonicalPath(directory, metadata.CanonicalPayload, ".tds", ".tdsx")
		if err != nil {
			return Item{}, err
		}
	case "flow":
		metadata, err := readFlowMetadata(directory)
		if err != nil {
			return Item{}, err
		}
		item.LUID, item.Name = metadata.TableauID, metadata.Name
		item.ServerOrigin, item.SiteLUID = metadata.SourceServerOrigin, metadata.SourceSiteLUID
		item.BaselineFingerprint = metadata.LocalBaselineFingerprint
		canonical, err = inventoryCanonicalPath(directory, metadata.CanonicalPayload, ".tfl", ".tflx")
		if err != nil {
			return Item{}, err
		}
		lineageData, err := readBoundedFile(filepath.Join(directory, metadata.LineageSidecar), maxLineageBytes)
		if err != nil {
			return Item{}, err
		}
		var lineage LineageDocument
		if err := json.Unmarshal(lineageData, &lineage); err != nil {
			return Item{}, err
		}
		if err := validateLineage(lineage); err != nil {
			return Item{}, err
		}
	default:
		return Item{}, errors.New("artifact kind is not supported")
	}
	relative, err := filepath.Rel(workspace, directory)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return Item{}, errors.New("artifact path escapes the workspace")
	}
	item.Path = filepath.ToSlash(relative)
	canonicalRelative, err := filepath.Rel(workspace, canonical)
	if err != nil || filepath.IsAbs(canonicalRelative) || strings.HasPrefix(canonicalRelative, "..") {
		return Item{}, errors.New("canonical artifact path escapes the workspace")
	}
	item.CanonicalPath = filepath.ToSlash(canonicalRelative)
	current, err := fingerprintFile(ctx, canonical)
	if errors.Is(err, os.ErrNotExist) {
		item.State = StateMissing
		item.TreeFingerprint, err = artifactTreeFingerprint(ctx, directory)
		if err != nil {
			return Item{}, err
		}
		return item, nil
	}
	if err != nil {
		return Item{}, err
	}
	item.CurrentFingerprint = current
	item.State = StateClean
	if current != item.BaselineFingerprint {
		item.State = StateDirty
	}
	item.TreeFingerprint, err = artifactTreeFingerprint(ctx, directory)
	if err != nil {
		return Item{}, err
	}
	return item, nil
}

func invalidArtifactWarning(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "metadata"):
		return "Artifact metadata failed validation."
	case strings.Contains(message, "canonical") || strings.Contains(message, "payload"):
		return "Artifact canonical payload failed validation."
	case strings.Contains(message, "lineage"):
		return "Artifact lineage sidecar failed validation."
	case strings.Contains(message, "symbolic link") || strings.Contains(message, "escapes"):
		return "Artifact containment failed validation."
	default:
		return "Managed artifact structure failed validation."
	}
}

func inventoryCanonicalPath(directory, payload string, extensions ...string) (string, error) {
	if payload == "" || filepath.IsAbs(payload) || filepath.Base(payload) != payload || filepath.Clean(payload) != payload {
		return "", errors.New("artifact metadata has an invalid canonical payload")
	}
	extension := strings.ToLower(filepath.Ext(payload))
	validExtension := false
	for _, candidate := range extensions {
		if extension == candidate {
			validExtension = true
			break
		}
	}
	if !validExtension {
		return "", errors.New("artifact metadata has an unsupported canonical payload")
	}
	return filepath.Join(directory, payload), nil
}

func validateWorkspaceRoot(workspace string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		return "", errors.New("workspace root is required")
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("workspace root must be a real directory")
	}
	if _, err := os.Lstat(filepath.Join(root, "tadx.yaml")); err != nil {
		return "", errors.New("workspace root does not contain tadx.yaml")
	}
	return root, nil
}

func validateArtifactDirectory(workspace, kind, directory string) error {
	root := filepath.Join(workspace, "artifacts", kind)
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("managed artifact must be a real directory")
	}
	relative, err := filepath.Rel(root, directory)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || strings.Contains(relative, string(filepath.Separator)) {
		return errors.New("artifact directory escapes its managed root")
	}
	return nil
}
