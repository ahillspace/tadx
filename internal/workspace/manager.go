// Package workspace owns named workspace registration and resolution.
package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	manifestVersion  = 1
	maxManifestBytes = 16 * 1024
	maxListLimit     = 10000
)

// Record is one registered workspace.
type Record struct {
	Name          string
	ID            string
	Root          string
	Default       bool
	Available     bool
	ManifestValid bool
}

// Page is one bounded registered-workspace page.
type Page struct {
	Items      []Record
	Returned   int
	Total      int
	Limit      int
	NextCursor string
}

// Manifest is the local identity stored at the workspace root.
type Manifest struct {
	Version   int               `yaml:"version" json:"version"`
	Workspace ManifestWorkspace `yaml:"workspace" json:"workspace"`
}

// ManifestWorkspace is the stable local workspace identity.
type ManifestWorkspace struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
}

// Manager owns named-workspace registration and resolution.
type Manager struct {
	configPath string
	random     io.Reader
}

// NewManager creates a workspace manager backed by one user configuration.
func NewManager(configPath string, random io.Reader) *Manager {
	if random == nil {
		random = rand.Reader
	}
	return &Manager{configPath: configPath, random: random}
}

// Create creates and registers one explicit named workspace.
func (m *Manager) Create(ctx context.Context, name, root string) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if m == nil || strings.TrimSpace(m.configPath) == "" {
		return Record{}, errors.New("workspace manager is not configured")
	}
	if err := config.ValidateWorkspaceName(name); err != nil {
		return Record{}, fmt.Errorf("workspace name: %w", err)
	}
	resolvedRoot, err := canonicalRoot(root)
	if err != nil {
		return Record{}, err
	}
	id, err := m.newID()
	if err != nil {
		return Record{}, fmt.Errorf("generate workspace identity: %w", err)
	}
	manifest := Manifest{Version: manifestVersion, Workspace: ManifestWorkspace{ID: id, Name: name}}
	created, err := createRoot(resolvedRoot, manifest)
	if err != nil {
		return Record{}, err
	}
	rollback := created
	defer func() {
		if rollback {
			_ = os.RemoveAll(resolvedRoot)
		}
	}()
	// Register under the interprocess configuration lock so the name/root
	// collision checks and the write are one atomic transaction; a concurrent
	// tadx process cannot slip a colliding registration between the check and
	// the save.
	updated, err := config.Update(m.configPath, true, func(configuration config.Config) (config.Config, error) {
		return applyRegistration(configuration, name, id, resolvedRoot)
	})
	if err != nil {
		return Record{}, err
	}
	rollback = false
	return Record{Name: name, ID: id, Root: resolvedRoot, Default: strings.EqualFold(updated.DefaultWorkspace, name), Available: true, ManifestValid: true}, nil
}

// Register adopts one existing on-disk workspace into the registry using the
// identity already recorded in its manifest. It creates nothing on disk: a
// missing or invalid tadx.yaml is a hard error, because register never
// initializes a workspace, it only adopts one that already exists.
func (m *Manager) Register(ctx context.Context, name, root string) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if m == nil || strings.TrimSpace(m.configPath) == "" {
		return Record{}, errors.New("workspace manager is not configured")
	}
	resolvedRoot, err := canonicalRoot(root)
	if err != nil {
		return Record{}, err
	}
	manifest, err := ReadManifest(resolvedRoot)
	if err != nil {
		return Record{}, fmt.Errorf("%q is not a workspace (no valid tadx.yaml); create one first with tadx workspace create: %w", root, err)
	}
	if name == "" {
		name = manifest.Workspace.Name
	}
	if err := config.ValidateWorkspaceName(name); err != nil {
		return Record{}, fmt.Errorf("workspace name: %w", err)
	}
	id := manifest.Workspace.ID
	updated, err := config.Update(m.configPath, true, func(configuration config.Config) (config.Config, error) {
		return applyRegistration(configuration, name, id, resolvedRoot)
	})
	if err != nil {
		return Record{}, err
	}
	return Record{Name: name, ID: id, Root: resolvedRoot, Default: strings.EqualFold(updated.DefaultWorkspace, name), Available: true, ManifestValid: true}, nil
}

// SetDefault selects one available registered workspace as the global default.
func (m *Manager) SetDefault(ctx context.Context, name string) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if m == nil || strings.TrimSpace(m.configPath) == "" {
		return Record{}, errors.New("workspace manager is not configured")
	}
	resolved, err := m.Resolve(ctx, name, "")
	if err != nil {
		return Record{}, err
	}
	updated, err := config.Update(m.configPath, false, func(configuration config.Config) (config.Config, error) {
		registeredName, registration, ok := exactRegistration(configuration, resolved.Name)
		if !ok || registration.ID != resolved.ID || !samePath(registration.Path, resolved.Root) {
			return config.Config{}, errors.New("workspace registration changed during default selection")
		}
		if configuration.DefaultWorkspace == registeredName {
			return config.Config{}, config.ErrNoChange
		}
		configuration.DefaultWorkspace = registeredName
		return configuration, nil
	})
	if err != nil {
		return Record{}, err
	}
	resolved.Default = strings.EqualFold(updated.DefaultWorkspace, resolved.Name)
	return resolved, nil
}

// Unregister removes one exact registry entry without changing workspace files.
func (m *Manager) Unregister(ctx context.Context, name string) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if m == nil || strings.TrimSpace(m.configPath) == "" {
		return Record{}, errors.New("workspace manager is not configured")
	}
	var removed Record
	_, err := config.Update(m.configPath, false, func(configuration config.Config) (config.Config, error) {
		registeredName, registration, ok := exactRegistration(configuration, name)
		if !ok {
			return config.Config{}, fmt.Errorf("workspace %q is not registered", name)
		}
		if err := validateDefaultReferences(configuration, registeredName); err != nil {
			return config.Config{}, err
		}
		removed = recordFromRegistration(registeredName, registration, configuration.DefaultWorkspace)
		removeRegistration(&configuration, registeredName)
		return configuration, nil
	})
	return removed, err
}

// Delete removes one exact registered workspace root and its registry entry.
// The caller must pass a record obtained from Resolve so identity changes fail closed.
func (m *Manager) Delete(ctx context.Context, expected Record) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if m == nil || strings.TrimSpace(m.configPath) == "" {
		return Record{}, errors.New("workspace manager is not configured")
	}
	if expected.Name == "" || expected.ID == "" || expected.Root == "" {
		return Record{}, errors.New("workspace deletion requires an exact resolved identity")
	}
	var removed Record
	var tombstone string
	_, err := config.UpdateWithRollback(m.configPath, false, func(current config.Config) (config.Config, func() error, error) {
		if err := ctx.Err(); err != nil {
			return config.Config{}, nil, err
		}
		name, registration, exists := exactRegistration(current, expected.Name)
		if !exists || registration.ID != expected.ID {
			return config.Config{}, nil, errors.New("workspace registration changed during deletion")
		}
		root, err := canonicalRoot(registration.Path)
		if err != nil || !samePath(root, expected.Root) {
			return config.Config{}, nil, errors.New("workspace root changed before deletion")
		}
		if err := validateDeletionRoot(root, name, registration.ID); err != nil {
			return config.Config{}, nil, err
		}
		for otherName, other := range current.Workspaces {
			if strings.EqualFold(otherName, name) {
				continue
			}
			otherRoot, resolveErr := canonicalRoot(other.Path)
			if containsPath(root, other.Path) || (resolveErr == nil && containsPath(root, otherRoot)) {
				return config.Config{}, nil, fmt.Errorf("workspace %q contains registered workspace %q", name, otherName)
			}
		}
		if err := validateDefaultReferences(current, name); err != nil {
			return config.Config{}, nil, err
		}
		removed = recordFromRegistration(name, registration, current.DefaultWorkspace)
		tombstone = filepath.Join(filepath.Dir(root), ".tadx-workspace-delete-"+registration.ID)
		if _, err := os.Lstat(tombstone); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				err = errors.New("workspace deletion staging path already exists")
			}
			return config.Config{}, nil, err
		}
		if err := os.Rename(root, tombstone); err != nil {
			return config.Config{}, nil, fmt.Errorf("stage workspace deletion: %w", err)
		}
		removeRegistration(&current, name)
		return current, func() error { return os.Rename(tombstone, root) }, nil
	})
	if err != nil {
		return Record{}, err
	}
	if err := os.RemoveAll(tombstone); err != nil {
		return Record{}, fmt.Errorf("workspace was unregistered but staged files remain at %q: %w", tombstone, err)
	}
	return removed, nil
}

func exactRegistration(configuration config.Config, selector string) (string, config.WorkspaceRegistration, bool) {
	if strings.TrimSpace(selector) == "" {
		return "", config.WorkspaceRegistration{}, false
	}
	for name, registration := range configuration.Workspaces {
		if strings.EqualFold(name, selector) {
			return name, registration, true
		}
	}
	return "", config.WorkspaceRegistration{}, false
}

func recordFromRegistration(name string, registration config.WorkspaceRegistration, defaultName string) Record {
	record := Record{Name: name, ID: registration.ID, Root: registration.Path, Default: strings.EqualFold(defaultName, name)}
	root, err := canonicalRoot(registration.Path)
	if err != nil {
		return record
	}
	record.Root = root
	manifest, err := ReadManifest(root)
	record.Available = err == nil
	record.ManifestValid = err == nil && manifest.Workspace.ID == registration.ID && strings.EqualFold(manifest.Workspace.Name, name)
	return record
}

func removeRegistration(configuration *config.Config, name string) {
	delete(configuration.Workspaces, name)
}

func validateDefaultReferences(configuration config.Config, name string) error {
	if strings.EqualFold(configuration.DefaultWorkspace, name) {
		return fmt.Errorf("workspace %q is the global default; reassign the default before removal", name)
	}
	var references []string
	for alias, environment := range configuration.Environments {
		if strings.EqualFold(environment.DefaultWorkspace, name) {
			references = append(references, alias)
		}
	}
	if len(references) > 0 {
		sort.Strings(references)
		return fmt.Errorf("workspace %q is the default for environment %q; reassign the default before removal", name, references[0])
	}
	return nil
}

func validateDeletionRoot(root, name, id string) error {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("workspace deletion target must be an available real directory")
	}
	volume := filepath.VolumeName(root) + string(filepath.Separator)
	if samePath(root, volume) || filepath.Dir(root) == root {
		return errors.New("workspace deletion target must not be a filesystem root")
	}
	manifest, err := ReadManifest(root)
	if err != nil || manifest.Workspace.ID != id || !strings.EqualFold(manifest.Workspace.Name, name) {
		return errors.New("workspace registry and manifest identities do not match")
	}
	return nil
}

// Clone copies one existing managed workspace to a new machine-local root under
// a freshly minted identity. Only the artifacts tree is carried over; the clone
// receives its own empty local state so nothing machine-specific leaks across.
func (m *Manager) Clone(ctx context.Context, source, newName, newRoot string) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if m == nil || strings.TrimSpace(m.configPath) == "" {
		return Record{}, errors.New("workspace manager is not configured")
	}
	if err := config.ValidateWorkspaceName(newName); err != nil {
		return Record{}, fmt.Errorf("workspace name: %w", err)
	}
	sourceRecord, err := m.Resolve(ctx, source, "")
	if err != nil {
		return Record{}, err
	}
	if _, err := ReadManifest(sourceRecord.Root); err != nil {
		return Record{}, fmt.Errorf("source workspace %q manifest: %w", sourceRecord.Name, err)
	}
	resolvedRoot, err := canonicalRoot(newRoot)
	if err != nil {
		return Record{}, err
	}
	if samePath(sourceRecord.Root, resolvedRoot) {
		return Record{}, errors.New("clone source and destination roots must differ")
	}
	id, err := m.newID()
	if err != nil {
		return Record{}, fmt.Errorf("generate workspace identity: %w", err)
	}
	manifest := Manifest{Version: manifestVersion, Workspace: ManifestWorkspace{ID: id, Name: newName}}
	if err := cloneRoot(ctx, sourceRecord.Root, resolvedRoot, manifest); err != nil {
		return Record{}, err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = os.RemoveAll(resolvedRoot)
		}
	}()
	updated, err := config.Update(m.configPath, true, func(configuration config.Config) (config.Config, error) {
		return applyRegistration(configuration, newName, id, resolvedRoot)
	})
	if err != nil {
		return Record{}, err
	}
	rollback = false
	return Record{Name: newName, ID: id, Root: resolvedRoot, Default: strings.EqualFold(updated.DefaultWorkspace, newName), Available: true, ManifestValid: true}, nil
}

// applyRegistration performs the name, identity, and root collision checks and
// installs one registration. It runs inside the config.Update lock so the check
// and the write are one atomic transaction against other tadx processes.
func applyRegistration(configuration config.Config, name, id, resolvedRoot string) (config.Config, error) {
	// Registration can wait for a deletion transaction after reading its
	// manifest. Reject a stale identity if that transaction removed the root.
	manifest, err := ReadManifest(resolvedRoot)
	if err != nil || manifest.Workspace.ID != id {
		return config.Config{}, errors.New("workspace manifest changed before registration")
	}
	for existingName, registration := range configuration.Workspaces {
		if strings.EqualFold(existingName, name) {
			return config.Config{}, fmt.Errorf("workspace name %q already exists", existingName)
		}
		if registration.ID == id {
			return config.Config{}, fmt.Errorf("workspace identity %q is already registered as %q", id, existingName)
		}
		existingRoot, rootErr := canonicalRoot(registration.Path)
		if rootErr != nil {
			// The registered root is currently unresolvable (for example an
			// unavailable volume). Fall back to a lexical comparison so an
			// offline workspace cannot block registering an unrelated one, while
			// still rejecting an exact duplicate registration.
			if samePath(registration.Path, resolvedRoot) {
				return config.Config{}, fmt.Errorf("workspace root is already registered as %q", existingName)
			}
			continue
		}
		if samePath(existingRoot, resolvedRoot) {
			return config.Config{}, fmt.Errorf("workspace root is already registered as %q", existingName)
		}
	}
	if configuration.Workspaces == nil {
		configuration.Workspaces = make(map[string]config.WorkspaceRegistration)
	}
	configuration.Workspaces[name] = config.WorkspaceRegistration{ID: id, Path: resolvedRoot}
	if configuration.DefaultWorkspace == "" {
		configuration.DefaultWorkspace = name
	}
	return configuration, nil
}

// Resolve resolves one logical name or the nearest configured default and validates its manifest.
func (m *Manager) Resolve(ctx context.Context, selector, environmentDefault string) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	configuration, err := config.Load(m.configPath)
	if err != nil {
		return Record{}, err
	}
	if selector == "" {
		selector = containingWorkspace(configuration)
		if selector == "" {
			selector = environmentDefault
		}
	}
	name, registration, err := configuration.ResolveWorkspace(selector)
	if err != nil {
		return Record{}, err
	}
	root, err := canonicalRoot(registration.Path)
	if err != nil {
		return Record{}, err
	}
	if _, err := upgradeLegacyManifest(root, Manifest{Version: manifestVersion, Workspace: ManifestWorkspace{ID: registration.ID, Name: name}}); err != nil {
		return Record{}, fmt.Errorf("workspace %q manifest migration: %w", name, err)
	}
	manifest, err := ReadManifest(root)
	if err != nil {
		return Record{}, fmt.Errorf("workspace %q manifest: %w", name, err)
	}
	if manifest.Workspace.ID != registration.ID || !strings.EqualFold(manifest.Workspace.Name, name) {
		return Record{}, fmt.Errorf("workspace %q registry and manifest identities do not match", name)
	}
	return Record{Name: name, ID: registration.ID, Root: root, Default: strings.EqualFold(configuration.DefaultWorkspace, name), Available: true, ManifestValid: true}, nil
}

func containingWorkspace(configuration config.Config) string {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return ""
	}
	workingDirectory, err = canonicalRoot(workingDirectory)
	if err != nil {
		return ""
	}
	type candidate struct {
		name  string
		depth int
	}
	var matches []candidate
	for name, registration := range configuration.Workspaces {
		root, rootErr := canonicalRoot(registration.Path)
		if rootErr != nil || !containsPath(root, workingDirectory) {
			continue
		}
		matches = append(matches, candidate{name: name, depth: len(filepath.Clean(root))})
	}
	if len(matches) == 0 {
		return ""
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].depth != matches[j].depth {
			return matches[i].depth > matches[j].depth
		}
		return strings.ToLower(matches[i].name) < strings.ToLower(matches[j].name)
	})
	return matches[0].name
}

func containsPath(root, target string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

// List returns a deterministic bounded page from the configured registry.
func (m *Manager) List(ctx context.Context, limit, offset int) (Page, error) {
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	if limit <= 0 || limit > maxListLimit {
		return Page{}, fmt.Errorf("workspace list limit must be between 1 and %d", maxListLimit)
	}
	if offset < 0 {
		return Page{}, errors.New("workspace list offset must not be negative")
	}
	configuration, err := config.Load(m.configPath)
	if err != nil {
		return Page{}, err
	}
	items := make([]Record, 0, len(configuration.Workspaces))
	for name, registration := range configuration.Workspaces {
		record := Record{Name: name, ID: registration.ID, Root: registration.Path, Default: strings.EqualFold(configuration.DefaultWorkspace, name)}
		root, rootErr := canonicalRoot(registration.Path)
		if rootErr == nil {
			record.Root = root
			_, _ = upgradeLegacyManifest(root, Manifest{Version: manifestVersion, Workspace: ManifestWorkspace{ID: registration.ID, Name: name}})
			manifest, manifestErr := ReadManifest(root)
			record.Available = manifestErr == nil
			record.ManifestValid = manifestErr == nil && manifest.Workspace.ID == registration.ID && strings.EqualFold(manifest.Workspace.Name, name)
		}
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool {
		left, right := strings.ToLower(items[i].Name), strings.ToLower(items[j].Name)
		if left == right {
			return items[i].ID < items[j].ID
		}
		return left < right
	})
	page := Page{Total: len(items), Limit: limit}
	if offset >= len(items) {
		return page, nil
	}
	end := min(offset+limit, len(items))
	page.Items = items[offset:end]
	page.Returned = len(page.Items)
	if end < len(items) {
		page.NextCursor = fmt.Sprintf("%d", end)
	}
	return page, nil
}

func upgradeLegacyManifest(root string, replacement Manifest) (bool, error) {
	path := filepath.Join(root, config.WorkspaceConfigName)
	data, mode, err := readBoundedManifest(path)
	if err != nil {
		return false, nil
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var legacy struct {
		Version   int                `yaml:"version"`
		Workspace *ManifestWorkspace `yaml:"workspace,omitempty"`
	}
	if err := decoder.Decode(&legacy); err != nil || legacy.Version != manifestVersion || legacy.Workspace != nil {
		return false, nil
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return false, nil
	}

	replacementData, err := yaml.Marshal(replacement)
	if err != nil {
		return false, err
	}
	temporary, err := os.CreateTemp(root, ".tadx-manifest-migration-*.yaml")
	if err != nil {
		return false, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	permissions := mode.Perm()
	if permissions == 0 {
		permissions = 0o600
	}
	if err := temporary.Chmod(permissions); err != nil {
		temporary.Close()
		return false, err
	}
	if _, err := temporary.Write(replacementData); err != nil {
		temporary.Close()
		return false, err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return false, err
	}
	if err := temporary.Close(); err != nil {
		return false, err
	}
	current, _, err := readBoundedManifest(path)
	if err != nil {
		return false, err
	}
	if !bytes.Equal(current, data) {
		return false, errors.New("workspace manifest changed during migration")
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return false, err
	}
	return true, nil
}

func readBoundedManifest(path string) ([]byte, os.FileMode, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxManifestBytes {
		return nil, 0, errors.New("workspace manifest must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	closeErr := file.Close()
	if err != nil {
		return nil, 0, err
	}
	if closeErr != nil {
		return nil, 0, closeErr
	}
	if len(data) > maxManifestBytes {
		return nil, 0, errors.New("workspace manifest exceeds its byte limit")
	}
	return data, info.Mode(), nil
}

// ReadManifest reads and validates one bounded workspace manifest.
func ReadManifest(root string) (Manifest, error) {
	path := filepath.Join(root, config.WorkspaceConfigName)
	data, _, err := readBoundedManifest(path)
	if err != nil {
		return Manifest{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.Version != manifestVersion || manifest.Workspace.ID == "" || manifest.Workspace.Name == "" {
		return Manifest{}, errors.New("workspace manifest identity is incomplete")
	}
	return manifest, nil
}

func (m *Manager) newID() (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(m.random, value); err != nil {
		return "", err
	}
	return "ws_" + hex.EncodeToString(value), nil
}

func createRoot(root string, manifest Manifest) (bool, error) {
	if _, err := os.Lstat(root); err == nil {
		return false, fmt.Errorf("workspace root %q must not already exist; use a new path for create or tadx workspace register for an existing workspace", root)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return false, err
	}
	stage, err := os.MkdirTemp(parent, ".tadx-workspace-stage-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(stage)
	if err := os.MkdirAll(filepath.Join(stage, "artifacts"), 0o700); err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Join(stage, ".tadx"), 0o700); err != nil {
		return false, err
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(filepath.Join(stage, config.WorkspaceConfigName), data, 0o600); err != nil {
		return false, err
	}
	if err := os.Rename(stage, root); err != nil {
		return false, fmt.Errorf("install workspace root: %w", err)
	}
	return true, nil
}

// cloneRoot builds a fresh workspace root from an existing source. It mirrors
// createRoot's atomic staged-then-renamed construction, copying only the source
// artifacts tree (rejecting symbolic links) and writing a fresh manifest and an
// empty local-state directory.
func cloneRoot(ctx context.Context, sourceRoot, root string, manifest Manifest) error {
	if _, err := os.Lstat(root); err == nil {
		return fmt.Errorf("workspace root %q already exists", root)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".tadx-workspace-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	stagedArtifacts := filepath.Join(stage, "artifacts")
	if err := os.MkdirAll(stagedArtifacts, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(stage, ".tadx"), 0o700); err != nil {
		return err
	}
	if err := copyManagedTree(ctx, filepath.Join(sourceRoot, "artifacts"), stagedArtifacts); err != nil {
		return err
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, config.WorkspaceConfigName), data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(stage, root); err != nil {
		return fmt.Errorf("install workspace root: %w", err)
	}
	return nil
}

// copyManagedTree recursively copies a managed directory, rejecting symbolic
// links and any non-regular entry. It mirrors the artifact manager's copy so a
// clone carries exactly the same containment guarantees as a move. A missing
// source directory copies as an empty tree.
func copyManagedTree(ctx context.Context, source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		from := filepath.Join(source, entry.Name())
		to := filepath.Join(destination, entry.Name())
		info, err := os.Lstat(from)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace entry %q must not be a symbolic link", entry.Name())
		}
		if info.IsDir() {
			if err := os.Mkdir(to, 0o700); err != nil {
				return err
			}
			if err := copyManagedTree(ctx, from, to); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("workspace entry %q must be a regular file or directory", entry.Name())
		}
		if err := copyManagedFile(ctx, from, to, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyManagedFile(ctx context.Context, source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, &contextReader{ctx: ctx, reader: input})
	if copyErr == nil {
		copyErr = output.Sync()
	}
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

func canonicalRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("workspace root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
		return resolved, nil
	}
	current := absolute
	var missing []string
	for {
		if _, statErr := os.Lstat(current); statErr == nil {
			break
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("inspect workspace parent: %w", statErr)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("workspace root has no existing ancestor")
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", fmt.Errorf("resolve workspace parent: %w", err)
	}
	for index := len(missing) - 1; index >= 0; index-- {
		resolved = filepath.Join(resolved, missing[index])
	}
	return resolved, nil
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if left == right {
		return true
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	if leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo) {
		return true
	}
	return false
}
