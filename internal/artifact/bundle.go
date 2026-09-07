package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	bundleStatePrepared   = "prepared"
	bundleStateCommitted  = "committed"
	maxBundleJournalBytes = 1024 * 1024
)

// WorkbookBundlePull describes one workbook and all direct published
// datasource siblings that must become visible as one local operation.
type WorkbookBundlePull struct {
	Workbook    WorkbookPull
	Datasources []DatasourcePull
}

// WorkbookBundlePullResult reports a committed workbook bundle.
type WorkbookBundlePullResult struct {
	Workbook    WorkbookPullResult
	Datasources []DatasourcePullResult
}

// WorkbookBundleManager owns atomic local persistence for a workbook and its
// acquired direct published datasource dependencies.
type WorkbookBundleManager struct {
	now        func() time.Time
	operations directoryOperations
}

// NewWorkbookBundleManager creates a workbook bundle manager.
func NewWorkbookBundleManager(now func() time.Time) *WorkbookBundleManager {
	if now == nil {
		now = time.Now
	}
	return &WorkbookBundleManager{now: now, operations: defaultDirectoryOperations()}
}

type preparedDirectory struct {
	target       string
	staging      string
	backupPrefix string
}

type bundleJournal struct {
	State   string               `json:"state"`
	Entries []bundleJournalEntry `json:"entries"`
}

type bundleJournalEntry struct {
	Target    string `json:"target"`
	Staging   string `json:"staging,omitempty"`
	Backup    string `json:"backup,omitempty"`
	HadTarget bool   `json:"had_target"`
}

// Pull preflights and stages every artifact before installing any artifact.
// If an installation fails, Pull restores every prior target in reverse order.
func (m *WorkbookBundleManager) Pull(ctx context.Context, input WorkbookBundlePull) (WorkbookBundlePullResult, error) {
	if err := ctx.Err(); err != nil {
		return WorkbookBundlePullResult{}, err
	}
	if len(input.Datasources) == 0 {
		return WorkbookBundlePullResult{}, errors.New("workbook bundle requires at least one datasource dependency")
	}
	workspace, err := filepath.Abs(input.Workbook.Workspace)
	if err != nil {
		return WorkbookBundlePullResult{}, err
	}
	if _, err := os.Stat(filepath.Join(workspace, "tadx.yaml")); err != nil {
		return WorkbookBundlePullResult{}, fmt.Errorf("workspace %q does not contain tadx.yaml", workspace)
	}
	handle, err := lockWorkspace(workspace)
	if err != nil {
		return WorkbookBundlePullResult{}, err
	}
	defer func() { _ = handle.Release() }()
	workbookRoot, err := ensureWorkbookRoot(workspace)
	if err != nil {
		return WorkbookBundlePullResult{}, err
	}
	datasourceRoot, err := ensureDatasourceRoot(workspace)
	if err != nil {
		return WorkbookBundlePullResult{}, err
	}
	operations := m.operations.withDefaults()
	if err := recoverBundleTransaction(workspace, operations); err != nil {
		return WorkbookBundlePullResult{}, err
	}
	workbookRecoveryWarnings, err := recoverWorkbookRoot(workbookRoot, operations)
	if err != nil {
		return WorkbookBundlePullResult{}, err
	}
	datasourceRecoveryWarnings, err := recoverDatasourceRoot(datasourceRoot, operations)
	if err != nil {
		return WorkbookBundlePullResult{}, err
	}

	prepared := make([]preparedDirectory, 0, len(input.Datasources)+1)
	defer func() {
		for _, item := range prepared {
			_ = operations.removeAll(item.staging)
		}
	}()

	datasourceResults := make([]DatasourcePullResult, 0, len(input.Datasources))
	pathsByLUID := make(map[string]string, len(input.Datasources))
	for _, datasource := range input.Datasources {
		if datasource.Overwrite {
			return WorkbookBundlePullResult{}, fmt.Errorf("datasource dependency %q cannot inherit workbook overwrite authorization", datasource.Metadata.TableauID)
		}
		datasourceWorkspace, absErr := filepath.Abs(datasource.Workspace)
		if absErr != nil {
			return WorkbookBundlePullResult{}, absErr
		}
		if datasourceWorkspace != workspace {
			return WorkbookBundlePullResult{}, errors.New("workbook bundle artifacts must use one workspace")
		}
		item, result, prepareErr := m.prepareDatasource(ctx, workspace, datasourceRoot, datasource, operations)
		if prepareErr != nil {
			return WorkbookBundlePullResult{}, prepareErr
		}
		prepared = append(prepared, item)
		luid := strings.TrimSpace(datasource.Metadata.TableauID)
		if _, exists := pathsByLUID[luid]; exists {
			return WorkbookBundlePullResult{}, fmt.Errorf("workbook bundle contains duplicate datasource LUID %q", luid)
		}
		datasourceResults = append(datasourceResults, result)
		pathsByLUID[luid] = result.WorkspaceRelativePath
	}
	if len(datasourceRecoveryWarnings) > 0 {
		datasourceResults[0].Warnings = append(datasourceRecoveryWarnings, datasourceResults[0].Warnings...)
	}

	references := make([]PublishedDatasourceRef, len(input.Workbook.Metadata.PublishedDatasources))
	copy(references, input.Workbook.Metadata.PublishedDatasources)
	seenReferences := make(map[string]struct{}, len(references))
	for index := range references {
		luid := strings.TrimSpace(references[index].LUID)
		if _, exists := seenReferences[luid]; exists {
			return WorkbookBundlePullResult{}, fmt.Errorf("workbook bundle contains duplicate published datasource reference %q", luid)
		}
		path, exists := pathsByLUID[luid]
		if !exists {
			return WorkbookBundlePullResult{}, fmt.Errorf("workbook bundle is missing datasource artifact %q", luid)
		}
		seenReferences[luid] = struct{}{}
		references[index].LocalArtifactPath = path
	}
	if len(seenReferences) != len(pathsByLUID) {
		return WorkbookBundlePullResult{}, errors.New("workbook bundle contains an unreferenced datasource artifact")
	}
	input.Workbook.Metadata.PublishedDatasources = references
	input.Workbook.Metadata.DependenciesAcquired = true
	workbookPrepared, workbookResult, err := m.prepareWorkbook(ctx, workspace, workbookRoot, input.Workbook, operations)
	if err != nil {
		return WorkbookBundlePullResult{}, err
	}
	prepared = append(prepared, workbookPrepared)
	workbookResult.Warnings = append(workbookRecoveryWarnings, workbookResult.Warnings...)

	commitWarnings, err := commitPreparedDirectories(workspace, prepared, operations)
	if err != nil {
		return WorkbookBundlePullResult{}, err
	}
	workbookResult.Warnings = append(workbookResult.Warnings, commitWarnings...)
	return WorkbookBundlePullResult{Workbook: workbookResult, Datasources: datasourceResults}, nil
}

func (m *WorkbookBundleManager) prepareDatasource(ctx context.Context, workspace, root string, input DatasourcePull, operations directoryOperations) (preparedDirectory, DatasourcePullResult, error) {
	if strings.TrimSpace(input.Metadata.TableauID) == "" || strings.TrimSpace(input.Metadata.Name) == "" {
		return preparedDirectory{}, DatasourcePullResult{}, errors.New("datasource artifact requires Tableau ID and name")
	}
	origin, err := NormalizeServerOrigin(input.Metadata.SourceServerOrigin)
	if err != nil {
		return preparedDirectory{}, DatasourcePullResult{}, errors.New(strings.NewReplacer("workbook artifact", "datasource artifact").Replace(err.Error()))
	}
	input.Metadata.SourceServerOrigin = origin
	input.Metadata.SourceSiteLUID = strings.TrimSpace(input.Metadata.SourceSiteLUID)
	if input.Metadata.SourceSiteLUID == "" {
		return preparedDirectory{}, DatasourcePullResult{}, errors.New("datasource artifact metadata requires source_site_luid")
	}
	nativeName := filepath.Base(input.Filename)
	nativeExtension := filepath.Ext(nativeName)
	extension := strings.ToLower(nativeExtension)
	if extension != ".tds" && extension != ".tdsx" {
		return preparedDirectory{}, DatasourcePullResult{}, fmt.Errorf("unsupported datasource artifact filename %q", input.Filename)
	}
	filename := portableComponent(strings.TrimSuffix(nativeName, nativeExtension), "datasource", maxPortableComponentBytes-len(nativeExtension)) + nativeExtension
	target, existing, err := findDatasourceBySourceIdentity(root, origin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID)
	if err != nil {
		return preparedDirectory{}, DatasourcePullResult{}, err
	}
	if target == "" {
		target = filepath.Join(root, datasourceIdentityComponent(input.Metadata.Name, origin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID))
		existing, err = inspectDatasourceTarget(root, target, origin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID)
		if err != nil {
			return preparedDirectory{}, DatasourcePullResult{}, err
		}
	}
	var warnings []string
	if existing != nil {
		canonical, pathErr := canonicalDatasourcePath(target, existing.CanonicalPayload)
		if pathErr != nil {
			return preparedDirectory{}, DatasourcePullResult{}, pathErr
		}
		current, fingerprintErr := fingerprintFile(ctx, canonical)
		if fingerprintErr != nil {
			return preparedDirectory{}, DatasourcePullResult{}, fmt.Errorf("fingerprint existing canonical datasource: %w", fingerprintErr)
		}
		if current != existing.LocalBaselineFingerprint {
			return preparedDirectory{}, DatasourcePullResult{}, fmt.Errorf("datasource artifact %q is dirty; dependency overwrite requires separate explicit authorization", target)
		}
		warnings = append(warnings, "clean datasource artifact refreshed from Tableau")
	}
	baseline := fingerprint(input.Content)
	metadata := input.Metadata
	metadata.Kind = "datasource"
	metadata.PulledAt = m.now().UTC().Format(time.RFC3339Nano)
	metadata.CanonicalPayload = filename
	metadata.LocalBaselineFingerprint = baseline
	metadata.CompositionStatus, metadata.ParentDataSourceURLs = classifyDatasourcePackage(filename, input.Content)
	metadata.sourceSitePresent = true
	if err := validateDatasourceMetadata(metadata); err != nil {
		return preparedDirectory{}, DatasourcePullResult{}, err
	}
	metadataBytes, err := encodeDatasourceMetadata(metadata)
	if err != nil {
		return preparedDirectory{}, DatasourcePullResult{}, err
	}
	staging, err := os.MkdirTemp(root, datasourceStagePrefix)
	if err != nil {
		return preparedDirectory{}, DatasourcePullResult{}, fmt.Errorf("create datasource artifact staging directory: %w", err)
	}
	if err := writeStagedArtifact(staging, map[string][]byte{filename: input.Content, "metadata.json": metadataBytes, "view.md": []byte(datasourceView(metadata))}, operations); err != nil {
		_ = operations.removeAll(staging)
		return preparedDirectory{}, DatasourcePullResult{}, err
	}
	relative, err := filepath.Rel(workspace, target)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		_ = operations.removeAll(staging)
		return preparedDirectory{}, DatasourcePullResult{}, fmt.Errorf("datasource artifact path %q escapes workspace %q", target, workspace)
	}
	return preparedDirectory{target: target, staging: staging, backupPrefix: datasourceBackupPrefix}, DatasourcePullResult{
		ArtifactPath: target, CanonicalPath: filepath.Join(target, filename), WorkspaceRelativePath: filepath.ToSlash(relative),
		BaselineFingerprint: baseline, Warnings: warnings, CompositionStatus: metadata.CompositionStatus,
		ParentDataSourceURLs: append([]string(nil), metadata.ParentDataSourceURLs...),
	}, nil
}

func inspectDatasourceTarget(root, target, origin, siteLUID, tableauID string) (*DatasourceMetadata, error) {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := validateDatasourceContainedPath(root, target); err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("artifact path %q is not a managed datasource directory", target)
	}
	metadata, err := readDatasourceMetadata(target)
	if err != nil {
		return nil, fmt.Errorf("artifact path %q is not a managed datasource artifact: %w", target, err)
	}
	if !sameDatasourceSourceIdentity(metadata, origin, siteLUID, tableauID) {
		return nil, fmt.Errorf("artifact path %q belongs to a different Tableau source identity", target)
	}
	if err := validateManagedDatasource(target, metadata); err != nil {
		return nil, fmt.Errorf("artifact path %q is not a managed datasource artifact: %w", target, err)
	}
	return &metadata, nil
}

func (m *WorkbookBundleManager) prepareWorkbook(ctx context.Context, workspace, root string, input WorkbookPull, operations directoryOperations) (preparedDirectory, WorkbookPullResult, error) {
	if strings.TrimSpace(input.Metadata.TableauID) == "" || strings.TrimSpace(input.Metadata.Name) == "" {
		return preparedDirectory{}, WorkbookPullResult{}, errors.New("workbook artifact requires Tableau ID and name")
	}
	origin, err := NormalizeServerOrigin(input.Metadata.SourceServerOrigin)
	if err != nil {
		return preparedDirectory{}, WorkbookPullResult{}, err
	}
	input.Metadata.SourceServerOrigin = origin
	input.Metadata.SourceSiteLUID = strings.TrimSpace(input.Metadata.SourceSiteLUID)
	if input.Metadata.SourceSiteLUID == "" {
		return preparedDirectory{}, WorkbookPullResult{}, errors.New("workbook artifact metadata requires source_site_luid")
	}
	nativeName := filepath.Base(input.Filename)
	nativeExtension := filepath.Ext(nativeName)
	extension := strings.ToLower(nativeExtension)
	if extension != ".twb" && extension != ".twbx" {
		return preparedDirectory{}, WorkbookPullResult{}, fmt.Errorf("unsupported workbook artifact filename %q", input.Filename)
	}
	filename := portableComponent(strings.TrimSuffix(nativeName, nativeExtension), "workbook", maxPortableComponentBytes-len(nativeExtension)) + nativeExtension
	target, existing, err := findBySourceIdentity(root, origin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID)
	if err != nil {
		return preparedDirectory{}, WorkbookPullResult{}, err
	}
	if target == "" {
		target = filepath.Join(root, identityComponent(input.Metadata.Name, origin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID))
		existing, err = inspectWorkbookTarget(root, target, origin, input.Metadata.SourceSiteLUID, input.Metadata.TableauID)
		if err != nil {
			return preparedDirectory{}, WorkbookPullResult{}, err
		}
	}
	var warnings []string
	if existing != nil {
		canonical, pathErr := canonicalWorkbookPath(target, existing.CanonicalPayload)
		if pathErr != nil {
			return preparedDirectory{}, WorkbookPullResult{}, pathErr
		}
		current, fingerprintErr := fingerprintFile(ctx, canonical)
		if fingerprintErr != nil {
			return preparedDirectory{}, WorkbookPullResult{}, fmt.Errorf("fingerprint existing canonical workbook: %w", fingerprintErr)
		}
		dirty := current != existing.LocalBaselineFingerprint
		if dirty && !input.Overwrite {
			return preparedDirectory{}, WorkbookPullResult{}, fmt.Errorf("workbook artifact %q is dirty; use --overwrite to replace local edits", target)
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
	lineage, err := applyWorkbookLineageMetadata(&metadata, input.Lineage, input.LineageCountsKnown)
	if err != nil {
		return preparedDirectory{}, WorkbookPullResult{}, err
	}
	if err := validateWorkbookMetadata(metadata); err != nil {
		return preparedDirectory{}, WorkbookPullResult{}, err
	}
	metadataBytes, err := encodeWorkbookMetadata(metadata)
	if err != nil {
		return preparedDirectory{}, WorkbookPullResult{}, err
	}
	lineageBytes, err := encodeWorkbookLineage(lineage)
	if err != nil {
		return preparedDirectory{}, WorkbookPullResult{}, err
	}
	staging, err := os.MkdirTemp(root, ".tadx-workbook-stage-")
	if err != nil {
		return preparedDirectory{}, WorkbookPullResult{}, fmt.Errorf("create artifact staging directory: %w", err)
	}
	if err := writeStagedArtifact(staging, map[string][]byte{filename: input.Content, "metadata.json": metadataBytes, "lineage.json": lineageBytes, "view.md": []byte(workbookView(metadata))}, operations); err != nil {
		_ = operations.removeAll(staging)
		return preparedDirectory{}, WorkbookPullResult{}, err
	}
	return preparedDirectory{target: target, staging: staging, backupPrefix: ".tadx-workbook-backup-"}, WorkbookPullResult{
		ArtifactPath: target, CanonicalPath: filepath.Join(target, filename), LineagePath: filepath.Join(target, "lineage.json"), LineageStatus: metadata.LineageStatus, BaselineFingerprint: baseline, Warnings: warnings,
	}, nil
}

func inspectWorkbookTarget(root, target, origin, siteLUID, tableauID string) (*WorkbookMetadata, error) {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := validateContainedPath(root, target); err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("artifact path %q is not a managed workbook directory", target)
	}
	metadata, err := readMetadata(target)
	if err != nil {
		return nil, fmt.Errorf("artifact path %q is not a managed workbook artifact: %w", target, err)
	}
	if !sameSourceIdentity(metadata, origin, siteLUID, tableauID) {
		return nil, fmt.Errorf("artifact path %q belongs to a different Tableau source identity", target)
	}
	if err := validateManagedWorkbook(target, metadata); err != nil {
		return nil, fmt.Errorf("artifact path %q is not a managed workbook artifact: %w", target, err)
	}
	return &metadata, nil
}

type installedDirectory struct {
	preparedDirectory
	backup    string
	installed bool
}

func commitPreparedDirectories(workspace string, prepared []preparedDirectory, operations directoryOperations) ([]string, error) {
	operations = operations.withDefaults()
	seen := make(map[string]struct{}, len(prepared))
	for _, item := range prepared {
		key := strings.ToLower(filepath.Clean(item.target))
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("artifact transaction contains duplicate target %q", item.target)
		}
		seen[key] = struct{}{}
	}
	journal := bundleJournal{State: bundleStatePrepared, Entries: make([]bundleJournalEntry, len(prepared))}
	for index, item := range prepared {
		entry := bundleJournalEntry{Target: item.target, Staging: item.staging}
		if _, err := operations.stat(item.target); err == nil {
			entry.HadTarget = true
			entry.Backup = filepath.Join(filepath.Dir(item.target), item.backupPrefix+filepath.Base(item.target)+"-"+uniqueSuffix())
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		journal.Entries[index] = entry
	}
	if err := writeBundleJournal(workspace, journal, operations); err != nil {
		return nil, err
	}
	installed := make([]installedDirectory, 0, len(prepared))
	rollback := func(cause error) error {
		if err := recoverBundleTransaction(workspace, operations); err != nil {
			return fmt.Errorf("%w; artifact transaction rollback failed: %v", cause, err)
		}
		return cause
	}
	for index, item := range prepared {
		record := installedDirectory{preparedDirectory: item}
		entry := journal.Entries[index]
		if entry.HadTarget {
			record.backup = entry.Backup
			if err := operations.rename(item.target, record.backup); err != nil {
				return nil, rollback(fmt.Errorf("stage existing artifact %q: %w", item.target, err))
			}
			installed = append(installed, record)
		} else {
			installed = append(installed, record)
		}
		if err := operations.rename(item.staging, item.target); err != nil {
			return nil, rollback(fmt.Errorf("install artifact %q: %w", item.target, err))
		}
		installed[len(installed)-1].installed = true
	}
	parents := make(map[string]struct{})
	for _, item := range installed {
		parents[filepath.Dir(item.target)] = struct{}{}
	}
	orderedParents := make([]string, 0, len(parents))
	for parent := range parents {
		orderedParents = append(orderedParents, parent)
	}
	sort.Strings(orderedParents)
	for _, parent := range orderedParents {
		if err := operations.syncDir(parent); err != nil {
			return nil, rollback(fmt.Errorf("sync artifact transaction root %q: %w", parent, err))
		}
	}
	journal.State = bundleStateCommitted
	if err := writeBundleJournal(workspace, journal, operations); err != nil {
		return nil, rollback(fmt.Errorf("mark artifact transaction committed: %w", err))
	}
	var warnings []string
	for _, item := range installed {
		if item.backup == "" {
			continue
		}
		if err := operations.removeAll(item.backup); err != nil {
			warnings = append(warnings, fmt.Sprintf("artifact transaction committed, but backup %q could not be removed: %v", item.backup, err))
		}
	}
	if len(warnings) == 0 {
		if err := removeBundleJournal(workspace, operations); err != nil {
			warnings = append(warnings, fmt.Sprintf("artifact transaction committed, but its recovery journal could not be removed: %v", err))
		}
	}
	return warnings, nil
}

func bundleJournalPath(workspace string) string {
	return filepath.Join(workspace, "artifacts", ".tadx-bundle-transaction.prepared.json")
}

func bundleCommittedPath(workspace string) string {
	return filepath.Join(workspace, "artifacts", ".tadx-bundle-transaction.committed.json")
}

func writeBundleJournal(workspace string, journal bundleJournal, operations directoryOperations) error {
	operations = operations.withDefaults()
	if journal.State != bundleStatePrepared && journal.State != bundleStateCommitted {
		return fmt.Errorf("invalid artifact transaction state %q", journal.State)
	}
	if err := validateBundleJournal(workspace, journal); err != nil {
		return err
	}
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return fmt.Errorf("encode artifact transaction journal: %w", err)
	}
	data = append(data, '\n')
	if len(data) > maxBundleJournalBytes {
		return fmt.Errorf("artifact transaction journal exceeds %d-byte limit", maxBundleJournalBytes)
	}
	root := filepath.Join(workspace, "artifacts")
	destination := bundleJournalPath(workspace)
	if journal.State == bundleStateCommitted {
		destination = bundleCommittedPath(workspace)
	}
	temporary := filepath.Join(root, ".tadx-bundle-journal-"+uniqueSuffix())
	if err := operations.writeFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write artifact transaction journal: %w", err)
	}
	defer operations.removeAll(temporary)
	// Link the fully flushed temporary file into place without replacement.
	// A concurrent caller therefore fails instead of stealing recovery ownership.
	if err := operations.link(temporary, destination); err != nil {
		return fmt.Errorf("install artifact transaction journal: %w", err)
	}
	if err := operations.syncDir(root); err != nil {
		return fmt.Errorf("sync artifact transaction journal: %w", err)
	}
	return nil
}

func recoverBundleTransaction(workspace string, operations directoryOperations) error {
	operations = operations.withDefaults()
	path := bundleCommittedPath(workspace)
	committed := true
	info, err := operations.stat(path)
	if errors.Is(err, os.ErrNotExist) {
		committed = false
		path = bundleJournalPath(workspace)
		info, err = operations.stat(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBundleJournalBytes {
		return fmt.Errorf("artifact transaction journal is invalid or exceeds %d-byte limit", maxBundleJournalBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxBundleJournalBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return fmt.Errorf("read artifact transaction journal: %w", readErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if len(data) > maxBundleJournalBytes {
		return fmt.Errorf("artifact transaction journal exceeds %d-byte limit", maxBundleJournalBytes)
	}
	var journal bundleJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		// Clean up the detached marker written by the earlier journal protocol.
		// It contains no paths and cannot authorize changes to live artifacts.
		if committed && string(data) == "committed\n" {
			if removeErr := operations.removeAll(bundleCommittedPath(workspace)); removeErr != nil {
				return removeErr
			}
			return operations.syncDir(filepath.Join(workspace, "artifacts"))
		}
		return fmt.Errorf("decode artifact transaction journal: %w", err)
	}
	if err := validateBundleJournal(workspace, journal); err != nil {
		return err
	}
	if committed && journal.State != bundleStateCommitted {
		return errors.New("committed artifact transaction journal has invalid state")
	}
	if !committed && journal.State != bundleStatePrepared {
		return errors.New("prepared artifact transaction journal has invalid state")
	}
	parents := make(map[string]struct{})
	for index := len(journal.Entries) - 1; index >= 0; index-- {
		entry := journal.Entries[index]
		parents[filepath.Dir(entry.Target)] = struct{}{}
		if committed {
			if entry.Backup != "" {
				if err := operations.removeAll(entry.Backup); err != nil {
					return fmt.Errorf("remove committed artifact backup %q: %w", entry.Backup, err)
				}
			}
		} else if entry.HadTarget {
			if _, err := operations.stat(entry.Backup); err == nil {
				if err := operations.removeAll(entry.Target); err != nil {
					return fmt.Errorf("remove partial artifact %q: %w", entry.Target, err)
				}
				if err := operations.rename(entry.Backup, entry.Target); err != nil {
					return fmt.Errorf("restore prior artifact %q: %w", entry.Target, err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		} else {
			if err := operations.removeAll(entry.Target); err != nil {
				return fmt.Errorf("remove partial new artifact %q: %w", entry.Target, err)
			}
		}
		if entry.Staging != "" {
			if err := operations.removeAll(entry.Staging); err != nil {
				return fmt.Errorf("remove artifact transaction stage %q: %w", entry.Staging, err)
			}
		}
	}
	for parent := range parents {
		if err := operations.syncDir(parent); err != nil {
			return err
		}
	}
	return removeBundleJournal(workspace, operations)
}

func validateBundleJournal(workspace string, journal bundleJournal) error {
	if journal.State != bundleStatePrepared && journal.State != bundleStateCommitted {
		return fmt.Errorf("artifact transaction journal has invalid state %q", journal.State)
	}
	if len(journal.Entries) == 0 {
		return errors.New("artifact transaction journal has no entries")
	}
	artifactsRoot, err := filepath.Abs(filepath.Join(workspace, "artifacts"))
	if err != nil {
		return err
	}
	for _, entry := range journal.Entries {
		if err := validateBundleTransactionPath(artifactsRoot, entry.Target, false); err != nil {
			return err
		}
		if entry.HadTarget && entry.Backup == "" {
			return fmt.Errorf("artifact transaction target %q requires a backup path", entry.Target)
		}
		for _, path := range []string{entry.Staging, entry.Backup} {
			if path != "" {
				if err := validateBundleTransactionPath(artifactsRoot, path, true); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateBundleTransactionPath(artifactsRoot, path string, hidden bool) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	parent := filepath.Dir(absolute)
	if parent != filepath.Join(artifactsRoot, "workbook") && parent != filepath.Join(artifactsRoot, "datasource") {
		return fmt.Errorf("artifact transaction path %q is outside managed artifact roots", path)
	}
	if hidden && !strings.HasPrefix(filepath.Base(absolute), ".tadx-") {
		return fmt.Errorf("artifact transaction internal path %q is invalid", path)
	}
	return nil
}

func removeBundleJournal(workspace string, operations directoryOperations) error {
	root := filepath.Join(workspace, "artifacts")
	// Remove the prepared record first. The complete committed record remains
	// authoritative until cleanup has finished and it is removed last.
	if err := operations.removeAll(bundleJournalPath(workspace)); err != nil {
		return err
	}
	if err := operations.syncDir(root); err != nil {
		return err
	}
	if err := operations.removeAll(bundleCommittedPath(workspace)); err != nil {
		return err
	}
	return operations.syncDir(root)
}
