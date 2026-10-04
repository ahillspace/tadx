package workspace

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/config"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

// NewLocalService binds one workspace workflow owner to local mechanisms.
func NewLocalService(configPath func() string, userHomeDir func() (string, error), resolve func(context.Context, string, string) (workspacecore.Record, error)) *Service {
	ports := &store{ConfigPath: configPath, UserHomeDir: userHomeDir, Resolve: resolve}
	return &Service{
		Creator: ports, Registrar: ports, Cloner: ports, Lister: ports,
		Reader: ports, Mover: ports, ArtifactStore: ports, Cleaner: ports,
		Setter: ports, Registry: ports, WorkspaceStore: ports,
	}
}

// store translates native registry and artifact results for the workspace owner.
type store struct {
	ConfigPath  func() string
	UserHomeDir func() (string, error)
	Resolve     func(context.Context, string, string) (workspacecore.Record, error)
}

func (w *store) manager() *workspacecore.Manager {
	return workspacecore.NewManager(w.ConfigPath(), nil)
}

func (w *store) resolve(ctx context.Context, selector string) (workspacecore.Record, error) {
	return w.Resolve(ctx, selector, "")
}

func (a *store) Create(ctx context.Context, input CreateInput) (Registration, error) {
	root, err := a.creationRoot(input.Name, input.Path)
	if err != nil {
		return Registration{}, err
	}
	operation := a.manager().Create
	if input.Preview {
		operation = a.manager().PreviewCreate
	}
	item, err := operation(ctx, input.Name, root)
	return workspaceRegistration(item), err
}

func (a *store) Register(ctx context.Context, input RegisterInput) (Registration, error) {
	operation := a.manager().Register
	if input.Preview {
		operation = a.manager().PreviewRegister
	}
	item, err := operation(ctx, input.Name, input.Path)
	return workspaceRegistration(item), err
}

func (a *store) Clone(ctx context.Context, input CloneInput) (Registration, error) {
	root, err := a.creationRoot(input.Name, input.Path)
	if err != nil {
		return Registration{}, err
	}
	operation := a.manager().Clone
	if input.Preview {
		operation = a.manager().PreviewClone
	}
	item, err := operation(ctx, input.Source, input.Name, root)
	return workspaceRegistration(item), err
}

func (w *store) creationRoot(name, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if err := config.ValidateWorkspaceName(name); err != nil {
		return "", fmt.Errorf("workspace name: %w", err)
	}
	home, err := w.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", errors.New("resolve user home directory: path is empty")
	}
	return filepath.Join(home, "TADX", "workspaces", name), nil
}

func (a *store) List(ctx context.Context, limit int, cursor string) (ListPage, error) {
	offset := 0
	if cursor != "" {
		value, err := strconv.Atoi(cursor)
		if err != nil || value < 0 {
			return ListPage{}, fmt.Errorf("workspace cursor must be a non-negative integer")
		}
		offset = value
	}
	page, err := a.manager().List(ctx, limit, offset)
	if err != nil {
		return ListPage{}, err
	}
	items := make([]ListedWorkspace, len(page.Items))
	for index, item := range page.Items {
		items[index] = ListedWorkspace{Workspace: workspaceIdentity(item), Default: item.Default, Available: item.Available, ManifestValid: item.ManifestValid}
	}
	return ListPage{Returned: page.Returned, Total: page.Total, Limit: page.Limit, NextCursor: page.NextCursor, Items: items}, nil
}

func (a *store) Status(ctx context.Context, input StatusInput) (Workspace, StatusInventory, error) {
	resolved, err := a.resolve(ctx, input.Workspace)
	if err != nil {
		return Workspace{}, StatusInventory{}, err
	}
	page, err := artifact.Inventory(ctx, resolved.Root, artifact.InventoryOptions{Limit: input.Limit, Cursor: input.Cursor})
	if err != nil {
		return Workspace{}, StatusInventory{}, err
	}
	items := make([]StatusArtifact, len(page.Items))
	for index, item := range page.Items {
		items[index] = statusArtifact(item)
	}
	return workspaceIdentity(resolved), StatusInventory{Returned: page.Returned, Total: page.Total, Limit: page.Limit, NextCursor: page.NextCursor, ScanComplete: page.ScanComplete, Clean: page.Clean, Dirty: page.Dirty, Missing: page.Missing, Invalid: page.Invalid, Items: items, Warnings: append([]string(nil), page.Warnings...)}, nil
}

func (a *store) Move(ctx context.Context, input MoveInput) (MoveArtifact, error) {
	source, err := a.resolve(ctx, input.SourceWorkspace)
	if err != nil {
		return MoveArtifact{}, err
	}
	destination, err := a.resolve(ctx, input.DestinationWorkspace)
	if err != nil {
		return MoveArtifact{}, err
	}
	operation := artifact.Move
	if input.Preview {
		operation = artifact.PreviewMove
	}
	item, err := operation(ctx, artifact.MoveRequest{SourceWorkspace: source.Root, DestinationWorkspace: destination.Root, Selector: artifact.Selector{Kind: input.Kind, LUID: input.LUID, Path: input.Path}})
	if err != nil {
		return moveArtifact(item), artifact.MapResolutionError("workspace.move", input.SourceWorkspace, input.LUID, err)
	}
	return moveArtifact(item), err
}

func (a *store) SetDefault(ctx context.Context, input SetDefaultInput) (Workspace, error) {
	operation := a.manager().SetDefault
	if input.Preview {
		operation = a.manager().PreviewSetDefault
	}
	item, err := operation(ctx, input.Name)
	return workspaceIdentity(item), err
}

func (a *store) Unregister(ctx context.Context, input UnregisterInput) (Workspace, error) {
	operation := a.manager().Unregister
	if input.Preview {
		operation = a.manager().PreviewUnregister
	}
	item, err := operation(ctx, input.Name)
	return workspaceIdentity(item), err
}

func (a *store) ResolveWorkspace(ctx context.Context, name string) (DeletionTarget, error) {
	item, err := a.manager().Resolve(ctx, name, "")
	if err != nil {
		return DeletionTarget{}, err
	}
	page, err := artifact.Inventory(ctx, item.Root, artifact.InventoryOptions{Limit: 1})
	if err != nil {
		return DeletionTarget{}, err
	}
	unmanaged, err := workspacecore.HasUnmanagedEntries(ctx, item.Root, page.ManagedPaths)
	if err != nil {
		return DeletionTarget{}, err
	}
	return DeletionTarget{Workspace: workspaceIdentity(item), Dirty: page.Dirty > 0 || page.Invalid > 0 || page.Missing > 0 || unmanaged, DirtyArtifacts: page.Dirty, InvalidArtifacts: page.Invalid + page.Missing}, nil
}
func (a *store) DeleteWorkspace(ctx context.Context, request WorkspaceDeleteRequest) error {
	current, err := a.ResolveWorkspace(ctx, request.Expected.Name)
	if err != nil {
		return err
	}
	if current.ID != request.Expected.ID || !workspacecore.SameRoot(current.Root, request.Expected.Root) {
		return errors.New("workspace identity changed during deletion revalidation")
	}
	if current.Dirty && !request.Force {
		return errors.New("workspace became dirty during deletion revalidation")
	}
	_, err = a.manager().Delete(ctx, workspacecore.Record{Name: current.Name, ID: current.ID, Root: current.Root})
	return err
}

func (a *store) Clean(ctx context.Context, request CleanInput) (CleanResult, error) {
	resolved, err := a.resolve(ctx, request.Workspace)
	if err != nil {
		return CleanResult{}, err
	}
	operation := workspacecore.Clean
	if request.Preview {
		operation = workspacecore.PreviewClean
	}
	result, err := operation(ctx, resolved.Root, request.Class)
	return CleanResult{EntriesRemoved: result.EntriesRemoved, BytesRemoved: result.BytesRemoved, Removed: result.Removed}, err
}

func (a *store) ResolveArtifact(ctx context.Context, input DeleteArtifactInput) (ArtifactTarget, error) {
	workspace, err := a.resolve(ctx, input.Workspace)
	if err != nil {
		return ArtifactTarget{}, err
	}
	item, err := artifact.Resolve(ctx, workspace.Root, artifact.Selector{Kind: input.Kind, LUID: input.LUID, Path: input.Path})
	if err != nil {
		return ArtifactTarget{}, artifact.MapResolutionError("workspace.artifact.delete", workspace.Name, input.LUID, err)
	}
	return deleteArtifact(item), err
}

func (a *store) DeleteArtifact(ctx context.Context, request ArtifactDeleteRequest) (ArtifactTarget, error) {
	workspace, err := a.resolve(ctx, request.Workspace)
	if err != nil {
		return ArtifactTarget{}, err
	}
	item, err := artifact.Delete(ctx, artifact.DeleteRequest{Workspace: workspace.Root, Expected: artifact.Item{Kind: request.Expected.Kind, LUID: request.Expected.LUID, Name: request.Expected.Name, Path: request.Expected.Path, CanonicalPath: request.Expected.CanonicalPath, State: request.Expected.State, ServerOrigin: request.Expected.ServerOrigin, SiteLUID: request.Expected.SiteLUID, BaselineFingerprint: request.Expected.BaselineFingerprint, CurrentFingerprint: request.Expected.CurrentFingerprint, TreeFingerprint: request.Expected.TreeFingerprint}})
	return deleteArtifact(item), err
}

func statusArtifact(item artifact.Item) StatusArtifact {
	diagnostic := ""
	if len(item.Warnings) > 0 {
		diagnostic = item.Warnings[0]
	}
	return StatusArtifact{Kind: item.Kind, LUID: item.LUID, Name: item.Name, Path: item.Path, State: item.State, Reason: item.Reason, Diagnostic: diagnostic, CanonicalPath: item.CanonicalPath, BaselineFingerprint: item.BaselineFingerprint, CurrentFingerprint: item.CurrentFingerprint}
}
func moveArtifact(item artifact.Item) MoveArtifact {
	return MoveArtifact{Kind: item.Kind, LUID: item.LUID, Name: item.Name, Path: item.Path, State: item.State, ServerOrigin: item.ServerOrigin, SiteLUID: item.SiteLUID, BaselineFingerprint: item.BaselineFingerprint, CurrentFingerprint: item.CurrentFingerprint, Warnings: append([]string(nil), item.Warnings...)}
}
func deleteArtifact(item artifact.Item) ArtifactTarget {
	return ArtifactTarget{Kind: item.Kind, LUID: item.LUID, Name: item.Name, Path: item.Path, State: item.State, CanonicalPath: item.CanonicalPath, ServerOrigin: item.ServerOrigin, SiteLUID: item.SiteLUID, BaselineFingerprint: item.BaselineFingerprint, CurrentFingerprint: item.CurrentFingerprint, TreeFingerprint: item.TreeFingerprint, Warnings: append([]string(nil), item.Warnings...)}
}

func workspaceIdentity(item workspacecore.Record) Workspace {
	return Workspace{Name: item.Name, ID: item.ID, Root: item.Root}
}

func workspaceRegistration(item workspacecore.Record) Registration {
	return Registration{Workspace: workspaceIdentity(item), ManifestVersion: 1, Registered: item.Available && item.ManifestValid}
}
