package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	artifactdelete "github.com/ahillspace/tadx/actions/workspace/artifact/delete"
	workspaceclean "github.com/ahillspace/tadx/actions/workspace/clean"
	workspaceclone "github.com/ahillspace/tadx/actions/workspace/clone"
	workspacecreate "github.com/ahillspace/tadx/actions/workspace/create"
	workspacedelete "github.com/ahillspace/tadx/actions/workspace/delete"
	workspacelist "github.com/ahillspace/tadx/actions/workspace/list"
	workspacemove "github.com/ahillspace/tadx/actions/workspace/move"
	workspaceregister "github.com/ahillspace/tadx/actions/workspace/register"
	workspacesetdefault "github.com/ahillspace/tadx/actions/workspace/setdefault"
	workspacestatus "github.com/ahillspace/tadx/actions/workspace/status"
	workspaceunregister "github.com/ahillspace/tadx/actions/workspace/unregister"
	"github.com/ahillspace/tadx/internal/artifact"
	workspacecli "github.com/ahillspace/tadx/internal/cli/workspace"
	"github.com/ahillspace/tadx/internal/config"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

type workspaceCommands struct {
	runtime         *workspaceRuntime
	create          *workspacecreate.Action
	register        *workspaceregister.Action
	clone           *workspaceclone.Action
	list            *workspacelist.Action
	status          *workspacestatus.Action
	move            *workspacemove.Action
	delete          *artifactdelete.Action
	clean           *workspaceclean.Action
	setDefault      *workspacesetdefault.Action
	unregister      *workspaceunregister.Action
	deleteWorkspace *workspacedelete.Action
}

func newWorkspaceCommands(runtime *runtimeDependencies) *workspaceCommands {
	shared := &workspaceRuntime{runtime: runtime}
	return &workspaceCommands{
		runtime: shared,
		create:  workspacecreate.New(workspaceCreator{shared}), register: workspaceregister.New(workspaceRegistrar{shared}),
		clone: workspaceclone.New(workspaceCloner{shared}), list: workspacelist.New(workspaceLister{shared}),
		status: workspacestatus.New(workspaceStatusReader{shared}), move: workspacemove.New(workspaceMover{shared}),
		delete:          artifactdelete.New(workspaceDeleteStore{shared}),
		clean:           workspaceclean.New(workspaceCleanStore{shared}),
		setDefault:      workspacesetdefault.New(workspaceDefaultStore{shared}),
		unregister:      workspaceunregister.New(workspaceRegistryStore{shared}),
		deleteWorkspace: workspacedelete.New(workspaceRootDeleteStore{shared}),
	}
}

func (c *workspaceCommands) dependencies() *workspacecli.Dependencies {
	ids := []string{"workspace.create", "workspace.register", "workspace.clone", "workspace.list", "workspace.status", "workspace.set-default", "workspace.unregister", "workspace.delete", "workspace.move", "workspace.artifact.delete", "workspace.clean"}
	return &workspacecli.Dependencies{Creator: c, Registrar: c, Cloner: c, Lister: c, Statuser: c, DefaultSetter: c, Unregistrar: c, WorkspaceDeleter: c, Mover: c, Deleter: c, Cleaner: c, Uses: registryUses(ids...), Shorts: registryShorts(ids...)}
}

func (c *workspaceCommands) Create(ctx context.Context, input workspacecreate.Input) (workspacecreate.Output, error) {
	return c.create.Execute(ctx, input)
}
func (c *workspaceCommands) Register(ctx context.Context, input workspaceregister.Input) (workspaceregister.Output, error) {
	return c.register.Execute(ctx, input)
}
func (c *workspaceCommands) Clone(ctx context.Context, input workspaceclone.Input) (workspaceclone.Output, error) {
	return c.clone.Execute(ctx, input)
}
func (c *workspaceCommands) List(ctx context.Context, input workspacelist.Input) (workspacelist.Output, error) {
	return c.list.Execute(ctx, input)
}
func (c *workspaceCommands) Status(ctx context.Context, input workspacestatus.Input) (workspacestatus.Output, error) {
	return c.status.Execute(ctx, input)
}
func (c *workspaceCommands) Move(ctx context.Context, input workspacemove.Input) (workspacemove.Output, error) {
	return c.move.Execute(ctx, input)
}
func (c *workspaceCommands) Delete(ctx context.Context, input artifactdelete.Input, preview bool) (artifactdelete.Output, error) {
	return c.delete.Execute(ctx, input, preview)
}
func (c *workspaceCommands) Clean(ctx context.Context, input workspaceclean.Input) (workspaceclean.Output, error) {
	return c.clean.Execute(ctx, input)
}
func (c *workspaceCommands) SetDefault(ctx context.Context, input workspacesetdefault.Input) (workspacesetdefault.Output, error) {
	return c.setDefault.Execute(ctx, input)
}
func (c *workspaceCommands) Unregister(ctx context.Context, input workspaceunregister.Input) (workspaceunregister.Output, error) {
	return c.unregister.Execute(ctx, input)
}
func (c *workspaceCommands) DeleteWorkspace(ctx context.Context, input workspacedelete.Input, preview bool) (workspacedelete.Output, error) {
	return c.deleteWorkspace.Execute(ctx, input, preview)
}

type workspaceRuntime struct{ runtime *runtimeDependencies }

func (w *workspaceRuntime) manager() *workspacecore.Manager {
	return workspacecore.NewManager(w.runtime.configPath, nil)
}

func (w *workspaceRuntime) resolve(ctx context.Context, selector string) (workspacecore.Record, error) {
	return w.resolveForEnvironment(ctx, selector, "")
}

func (w *workspaceRuntime) resolveForEnvironment(ctx context.Context, selector, environmentAlias string) (workspacecore.Record, error) {
	configuration, err := w.runtime.configuration()
	if err != nil {
		return workspacecore.Record{}, err
	}
	environmentDefault := ""
	if environmentAlias != "" || configuration.DefaultEnvironment != "" {
		environment, resolveErr := configuration.ResolveEnvironment(environmentAlias)
		if resolveErr != nil {
			return workspacecore.Record{}, resolveErr
		}
		environmentDefault = environment.DefaultWorkspace
	}
	return w.runtime.resolveWorkspace(ctx, configuration, selector, environmentDefault)
}

type workspaceCreator struct{ runtime *workspaceRuntime }

func (a workspaceCreator) Create(ctx context.Context, input workspacecreate.Input) (workspacecreate.Workspace, error) {
	root, err := a.runtime.creationRoot(input.Name, input.Path)
	if err != nil {
		return workspacecreate.Workspace{}, err
	}
	item, err := a.runtime.manager().Create(ctx, input.Name, root)
	return workspacecreate.Workspace{Name: item.Name, ID: item.ID, Root: item.Root, ManifestVersion: 1, Registered: item.Available && item.ManifestValid, CreatedEntries: []string{"tadx.yaml", "artifacts", ".tadx"}}, err
}

type workspaceRegistrar struct{ runtime *workspaceRuntime }

func (a workspaceRegistrar) Register(ctx context.Context, input workspaceregister.Input) (workspaceregister.Workspace, error) {
	item, err := a.runtime.manager().Register(ctx, input.Name, input.Path)
	return workspaceregister.Workspace{Name: item.Name, ID: item.ID, ManifestVersion: 1, Registered: item.Available && item.ManifestValid}, err
}

type workspaceCloner struct{ runtime *workspaceRuntime }

func (a workspaceCloner) Clone(ctx context.Context, input workspaceclone.Input) (workspaceclone.Workspace, error) {
	root, err := a.runtime.creationRoot(input.Name, input.Path)
	if err != nil {
		return workspaceclone.Workspace{}, err
	}
	item, err := a.runtime.manager().Clone(ctx, input.Source, input.Name, root)
	return workspaceclone.Workspace{Name: item.Name, ID: item.ID, Root: item.Root, ManifestVersion: 1, Registered: item.Available && item.ManifestValid, CreatedEntries: []string{"tadx.yaml", "artifacts", ".tadx"}}, err
}

func (w *workspaceRuntime) creationRoot(name, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if err := config.ValidateWorkspaceName(name); err != nil {
		return "", fmt.Errorf("workspace name: %w", err)
	}
	if w == nil || w.runtime == nil || w.runtime.userHomeDir == nil {
		return "", errors.New("user home directory resolution is not configured")
	}
	home, err := w.runtime.userHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", errors.New("resolve user home directory: path is empty")
	}
	return filepath.Join(home, "TADX", "workspaces", name), nil
}

type workspaceLister struct{ runtime *workspaceRuntime }

func (a workspaceLister) List(ctx context.Context, limit int, cursor string) (workspacelist.Page, error) {
	offset := 0
	if cursor != "" {
		value, err := strconv.Atoi(cursor)
		if err != nil || value < 0 {
			return workspacelist.Page{}, fmt.Errorf("workspace cursor must be a non-negative integer")
		}
		offset = value
	}
	page, err := a.runtime.manager().List(ctx, limit, offset)
	if err != nil {
		return workspacelist.Page{}, err
	}
	items := make([]workspacelist.Workspace, len(page.Items))
	for index, item := range page.Items {
		items[index] = workspacelist.Workspace{Name: item.Name, ID: item.ID, Root: item.Root, Default: item.Default, Available: item.Available, ManifestValid: item.ManifestValid}
	}
	return workspacelist.Page{Returned: page.Returned, Total: page.Total, Limit: page.Limit, NextCursor: page.NextCursor, Items: items}, nil
}

type workspaceStatusReader struct{ runtime *workspaceRuntime }

func (a workspaceStatusReader) Status(ctx context.Context, input workspacestatus.Input) (workspacestatus.Workspace, workspacestatus.Inventory, error) {
	resolved, err := a.runtime.resolve(ctx, input.Workspace)
	if err != nil {
		return workspacestatus.Workspace{}, workspacestatus.Inventory{}, err
	}
	page, err := artifact.Inventory(ctx, resolved.Root, artifact.InventoryOptions{Limit: input.Limit, Cursor: input.Cursor})
	if err != nil {
		return workspacestatus.Workspace{}, workspacestatus.Inventory{}, err
	}
	items := make([]workspacestatus.Artifact, len(page.Items))
	for index, item := range page.Items {
		items[index] = statusArtifact(item)
	}
	return workspacestatus.Workspace{Name: resolved.Name, ID: resolved.ID, Root: resolved.Root}, workspacestatus.Inventory{Returned: page.Returned, Total: page.Total, Limit: page.Limit, NextCursor: page.NextCursor, ScanComplete: page.ScanComplete, Clean: page.Clean, Dirty: page.Dirty, Missing: page.Missing, Invalid: page.Invalid, Items: items, Warnings: append([]string(nil), page.Warnings...)}, nil
}

type workspaceMover struct{ runtime *workspaceRuntime }

func (a workspaceMover) Move(ctx context.Context, input workspacemove.Input) (workspacemove.Artifact, error) {
	source, err := a.runtime.resolve(ctx, input.SourceWorkspace)
	if err != nil {
		return workspacemove.Artifact{}, err
	}
	destination, err := a.runtime.resolve(ctx, input.DestinationWorkspace)
	if err != nil {
		return workspacemove.Artifact{}, err
	}
	item, err := artifact.Move(ctx, artifact.MoveRequest{SourceWorkspace: source.Root, DestinationWorkspace: destination.Root, Selector: artifact.Selector{Kind: input.Kind, LUID: input.LUID, Path: input.Path}})
	return moveArtifact(item), err
}

type workspaceDeleteStore struct{ runtime *workspaceRuntime }

type workspaceCleanStore struct{ runtime *workspaceRuntime }

type workspaceDefaultStore struct{ runtime *workspaceRuntime }

func (a workspaceDefaultStore) SetDefault(ctx context.Context, name string) (workspacesetdefault.Workspace, error) {
	item, err := a.runtime.manager().SetDefault(ctx, name)
	return workspacesetdefault.Workspace{Name: item.Name, ID: item.ID, Root: item.Root}, err
}

type workspaceRegistryStore struct{ runtime *workspaceRuntime }

func (a workspaceRegistryStore) Unregister(ctx context.Context, name string) (workspaceunregister.Workspace, error) {
	item, err := a.runtime.manager().Unregister(ctx, name)
	return workspaceunregister.Workspace{Name: item.Name, ID: item.ID, Root: item.Root}, err
}

type workspaceRootDeleteStore struct{ runtime *workspaceRuntime }

func (a workspaceRootDeleteStore) Resolve(ctx context.Context, name string) (workspacedelete.Workspace, error) {
	item, err := a.runtime.manager().Resolve(ctx, name, "")
	if err != nil {
		return workspacedelete.Workspace{}, err
	}
	page, err := artifact.Inventory(ctx, item.Root, artifact.InventoryOptions{Limit: 1})
	if err != nil {
		return workspacedelete.Workspace{}, err
	}
	unmanaged, err := workspaceHasUnmanagedEntries(ctx, item.Root, page.ManagedPaths)
	if err != nil {
		return workspacedelete.Workspace{}, err
	}
	return workspacedelete.Workspace{Name: item.Name, ID: item.ID, Root: item.Root, Dirty: page.Dirty > 0 || page.Invalid > 0 || page.Missing > 0 || unmanaged, DirtyArtifacts: page.Dirty, InvalidArtifacts: page.Invalid + page.Missing}, nil
}
func (a workspaceRootDeleteStore) Delete(ctx context.Context, request workspacedelete.DeleteRequest) error {
	current, err := a.Resolve(ctx, request.Expected.Name)
	if err != nil {
		return err
	}
	if current.ID != request.Expected.ID || !sameWorkspaceRoot(current.Root, request.Expected.Root) {
		return errors.New("workspace identity changed during deletion revalidation")
	}
	if current.Dirty && !request.Force {
		return errors.New("workspace became dirty during deletion revalidation")
	}
	_, err = a.runtime.manager().Delete(ctx, workspacecore.Record{Name: current.Name, ID: current.ID, Root: current.Root})
	return err
}

func sameWorkspaceRoot(left, right string) bool {
	leftPath, leftErr := filepath.Abs(left)
	rightPath, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	leftPath, rightPath = filepath.Clean(leftPath), filepath.Clean(rightPath)
	if leftPath == rightPath {
		return true
	}
	leftInfo, leftErr := os.Stat(leftPath)
	rightInfo, rightErr := os.Stat(rightPath)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}

func workspaceHasUnmanagedEntries(ctx context.Context, root string, managedPaths []string) (bool, error) {
	const maxEntries = 10000
	entries := 0
	managedKinds := map[string]bool{"workbook": true, "datasource": true, "flow": true, "pulse-definition": true, "lineage": true}
	managedFiles := map[string]bool{}
	managedDirectories := map[string]bool{}
	for _, relative := range managedPaths {
		managedFiles[relative] = true
		for directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relative))); directory != "."; directory = filepath.ToSlash(filepath.Dir(filepath.FromSlash(directory))) {
			managedDirectories[directory] = true
		}
	}
	dirty := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entries++
		if entries > maxEntries {
			return errors.New("workspace deletion inspection exceeds its bounded entry limit")
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		parts := strings.Split(relative, "/")
		if entry.Type()&os.ModeSymlink != 0 {
			dirty = true
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		switch parts[0] {
		case config.WorkspaceConfigName, ".tadx.lock":
			if len(parts) != 1 || !entry.Type().IsRegular() {
				dirty = true
			}
		case ".tadx":
			if len(parts) == 1 {
				return nil
			}
		case "artifacts":
			if len(parts) == 1 && entry.IsDir() {
				return nil
			}
			if len(parts) >= 2 && managedKinds[parts[1]] {
				if len(parts) == 2 && entry.IsDir() {
					return nil
				}
				if parts[1] == "lineage" && len(parts) == 3 && entry.IsDir() && (parts[2] == "workbook" || parts[2] == "published_datasource" || parts[2] == "flow") {
					return nil
				}
				if managedDirectories[relative] && entry.IsDir() {
					return nil
				}
				if managedFiles[relative] && entry.Type().IsRegular() {
					return nil
				}
			}
			dirty = true
			if entry.IsDir() {
				return filepath.SkipDir
			}
		default:
			dirty = true
			if entry.IsDir() && len(parts) == 1 {
				return filepath.SkipDir
			}
		}
		return nil
	})
	return dirty, err
}

func (a workspaceCleanStore) Clean(ctx context.Context, request workspaceclean.Request) (workspaceclean.Result, error) {
	resolved, err := a.runtime.resolve(ctx, request.Workspace)
	if err != nil {
		return workspaceclean.Result{}, err
	}
	result, err := workspacecore.Clean(ctx, resolved.Root, request.Class)
	return workspaceclean.Result{EntriesRemoved: result.EntriesRemoved, BytesRemoved: result.BytesRemoved, Removed: result.Removed}, err
}

func (a workspaceDeleteStore) Resolve(ctx context.Context, input artifactdelete.Input) (artifactdelete.Artifact, error) {
	workspace, err := a.runtime.resolve(ctx, input.Workspace)
	if err != nil {
		return artifactdelete.Artifact{}, err
	}
	item, err := artifact.Resolve(ctx, workspace.Root, artifact.Selector{Kind: input.Kind, LUID: input.LUID, Path: input.Path})
	return deleteArtifact(item), err
}
func (a workspaceDeleteStore) Delete(ctx context.Context, request artifactdelete.DeleteRequest) (artifactdelete.Artifact, error) {
	workspace, err := a.runtime.resolve(ctx, request.Workspace)
	if err != nil {
		return artifactdelete.Artifact{}, err
	}
	item, err := artifact.Delete(ctx, artifact.DeleteRequest{Workspace: workspace.Root, Expected: artifact.Item{Kind: request.Expected.Kind, LUID: request.Expected.LUID, Name: request.Expected.Name, Path: request.Expected.Path, CanonicalPath: request.Expected.CanonicalPath, State: request.Expected.State, ServerOrigin: request.Expected.ServerOrigin, SiteLUID: request.Expected.SiteLUID, BaselineFingerprint: request.Expected.BaselineFingerprint, CurrentFingerprint: request.Expected.CurrentFingerprint, TreeFingerprint: request.Expected.TreeFingerprint}})
	return deleteArtifact(item), err
}

func statusArtifact(item artifact.Item) workspacestatus.Artifact {
	return workspacestatus.Artifact{Kind: item.Kind, LUID: item.LUID, Name: item.Name, Path: item.Path, State: item.State, CanonicalPath: item.CanonicalPath, BaselineFingerprint: item.BaselineFingerprint, CurrentFingerprint: item.CurrentFingerprint}
}
func moveArtifact(item artifact.Item) workspacemove.Artifact {
	return workspacemove.Artifact{Kind: item.Kind, LUID: item.LUID, Name: item.Name, Path: item.Path, State: item.State, ServerOrigin: item.ServerOrigin, SiteLUID: item.SiteLUID, BaselineFingerprint: item.BaselineFingerprint, CurrentFingerprint: item.CurrentFingerprint, Warnings: append([]string(nil), item.Warnings...)}
}
func deleteArtifact(item artifact.Item) artifactdelete.Artifact {
	return artifactdelete.Artifact{Kind: item.Kind, LUID: item.LUID, Name: item.Name, Path: item.Path, State: item.State, CanonicalPath: item.CanonicalPath, ServerOrigin: item.ServerOrigin, SiteLUID: item.SiteLUID, BaselineFingerprint: item.BaselineFingerprint, CurrentFingerprint: item.CurrentFingerprint, TreeFingerprint: item.TreeFingerprint, Warnings: append([]string(nil), item.Warnings...)}
}
