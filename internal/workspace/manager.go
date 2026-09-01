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
	maxListLimit     = 200
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
	configuration, err := config.Load(m.configPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Record{}, err
		}
		configuration = config.Config{Version: config.CurrentVersion}
	}
	for existingName := range configuration.Workspaces {
		if strings.EqualFold(existingName, name) {
			return Record{}, fmt.Errorf("workspace name %q already exists", existingName)
		}
	}
	resolvedRoot, err := canonicalRoot(root)
	if err != nil {
		return Record{}, err
	}
	for existingName, registration := range configuration.Workspaces {
		existingRoot, rootErr := canonicalRoot(registration.Path)
		if rootErr != nil {
			return Record{}, fmt.Errorf("registered workspace %q: %w", existingName, rootErr)
		}
		if samePath(existingRoot, resolvedRoot) {
			return Record{}, fmt.Errorf("workspace root is already registered as %q", existingName)
		}
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
	if configuration.Workspaces == nil {
		configuration.Workspaces = make(map[string]config.WorkspaceRegistration)
	}
	configuration.Workspaces[name] = config.WorkspaceRegistration{ID: id, Path: resolvedRoot}
	if configuration.DefaultWorkspace == "" {
		configuration.DefaultWorkspace = name
	}
	if err := config.Save(m.configPath, configuration); err != nil {
		return Record{}, fmt.Errorf("register workspace: %w", err)
	}
	rollback = false
	return Record{Name: name, ID: id, Root: resolvedRoot, Default: strings.EqualFold(configuration.DefaultWorkspace, name), Available: true, ManifestValid: true}, nil
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
		return false, fmt.Errorf("workspace root %q already exists", root)
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
