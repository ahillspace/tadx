package workspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahillspace/tadx/internal/config"
)

// PreviewCreate checks the proposed root and registry without allocating identity
// or creating files. The eventual write repeats its own collision checks.
func (m *Manager) PreviewCreate(ctx context.Context, name, root string) (Record, error) {
	if m == nil || strings.TrimSpace(m.configPath) == "" {
		return Record{}, errors.New("workspace manager is not configured")
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if err := config.ValidateWorkspaceName(name); err != nil {
		return Record{}, err
	}
	if info, err := os.Lstat(filepath.Clean(root)); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return Record{}, errors.New("workspace root must be a real directory")
	}
	resolved, err := canonicalRoot(root)
	if err != nil {
		return Record{}, err
	}
	if _, err := os.Lstat(resolved); err == nil {
		empty, err := emptyDirectory(resolved)
		if err != nil {
			return Record{}, err
		}
		if !empty {
			return Record{}, errors.New("workspace root must be new or empty")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Record{}, err
	}
	cfg, err := config.Load(m.configPath)
	if errors.Is(err, os.ErrNotExist) {
		cfg, err = config.Config{Version: config.CurrentVersion}, nil
	}
	if err != nil {
		return Record{}, err
	}
	if _, err := planRegistration(cfg, name, "", resolved); err != nil {
		return Record{}, err
	}
	return Record{Name: name, Root: resolved}, nil
}

func (m *Manager) PreviewRegister(ctx context.Context, name, root string) (Record, error) {
	if m == nil || strings.TrimSpace(m.configPath) == "" {
		return Record{}, errors.New("workspace manager is not configured")
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	resolved, err := canonicalRoot(root)
	if err != nil {
		return Record{}, err
	}
	manifest, err := ReadManifest(resolved)
	if err != nil {
		return Record{}, err
	}
	if name == "" {
		name = manifest.Workspace.Name
	}
	if err := config.ValidateWorkspaceName(name); err != nil {
		return Record{}, err
	}
	cfg, err := config.Load(m.configPath)
	if errors.Is(err, os.ErrNotExist) {
		cfg, err = config.Config{Version: config.CurrentVersion}, nil
	}
	if err != nil {
		return Record{}, err
	}
	if _, err := applyRegistration(cfg, name, manifest.Workspace.ID, resolved); err != nil {
		return Record{}, err
	}
	return Record{Name: name, ID: manifest.Workspace.ID, Root: resolved, Available: true, ManifestValid: true}, nil
}

func (m *Manager) PreviewClone(ctx context.Context, source, name, root string) (Record, error) {
	src, err := m.previewResolve(ctx, source)
	if err != nil {
		return Record{}, err
	}
	plan, err := m.PreviewCreate(ctx, name, root)
	if err != nil {
		return Record{}, err
	}
	if _, err := os.Lstat(plan.Root); !errors.Is(err, os.ErrNotExist) {
		return Record{}, errors.New("clone destination must not already exist")
	}
	if samePath(src.Root, plan.Root) {
		return Record{}, errors.New("clone source and destination roots must differ")
	}
	const maxEntries = 100000
	count := 0
	artifacts := filepath.Join(src.Root, "artifacts")
	err = filepath.WalkDir(artifacts, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) && path == artifacts {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		count++
		if count > maxEntries {
			return errors.New("clone preview exceeds 100000 entries")
		}
		if entry.Type()&os.ModeSymlink != 0 || (!entry.IsDir() && !entry.Type().IsRegular()) {
			return fmt.Errorf("workspace entry %q must be a real directory or regular file", entry.Name())
		}
		return nil
	})
	return plan, err
}

func (m *Manager) PreviewSetDefault(ctx context.Context, name string) (Record, error) {
	return m.previewResolve(ctx, name)
}

func (m *Manager) previewResolve(ctx context.Context, name string) (Record, error) {
	if m == nil || strings.TrimSpace(m.configPath) == "" {
		return Record{}, errors.New("workspace manager is not configured")
	}
	cfg, err := config.Load(m.configPath)
	if err != nil {
		return Record{}, err
	}
	return m.ResolveReadOnlyWithConfig(ctx, cfg, name, "")
}

func (m *Manager) PreviewUnregister(ctx context.Context, name string) (Record, error) {
	if m == nil || strings.TrimSpace(m.configPath) == "" {
		return Record{}, errors.New("workspace manager is not configured")
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	cfg, err := config.Load(m.configPath)
	if err != nil {
		return Record{}, err
	}
	registered, entry, ok := exactRegistration(cfg, name)
	if !ok {
		return Record{}, fmt.Errorf("workspace %q is not registered", name)
	}
	if err := validateDefaultReferences(cfg, registered); err != nil {
		return Record{}, err
	}
	return recordFromRegistration(registered, entry, cfg.DefaultWorkspace), nil
}
