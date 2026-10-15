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

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	workspacecli "github.com/ahillspace/tadx/internal/cli/workspace"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func newWorkspaceCommands(runtime *runtimeDependencies) *workspacecli.Dependencies {
	shared := &workspaceRuntime{runtime: runtime}
	service := &workspaceaction.Service{
		Creator: shared, Registrar: shared, Cloner: shared, Lister: shared,
		Reader: shared, Mover: shared, ArtifactStore: shared, Cleaner: shared,
		Setter: shared, Registry: shared, WorkspaceStore: shared,
	}
	ids := []string{"workspace.create", "workspace.register", "workspace.clone", "workspace.list", "workspace.status", "workspace.set-default", "workspace.unregister", "workspace.delete", "workspace.move", "workspace.artifact.delete", "workspace.clean"}
	return &workspacecli.Dependencies{Creator: service, Registrar: service, Cloner: service, Lister: service, Statuser: service, DefaultSetter: service, Unregistrar: service, WorkspaceDeleter: service, Mover: service, Deleter: service, Cleaner: service, Uses: registryUses(ids...), Shorts: registryShorts(ids...)}
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
	return w.runtime.resolveWorkspace(ctx, configuration, selector, environmentAlias)
}

func (a *workspaceRuntime) Create(ctx context.Context, input workspaceaction.CreateInput) (workspaceaction.Registration, error) {
	root, err := a.creationRoot(input.Name, input.Path)
	if err != nil {
		return workspaceaction.Registration{}, err
	}
	operation := a.manager().Create
	if input.Preview {
		operation = a.manager().PreviewCreate
	}
	item, err := operation(ctx, input.Name, root)
	return workspaceRegistration(item), err
}

func (a *workspaceRuntime) Register(ctx context.Context, input workspaceaction.RegisterInput) (workspaceaction.Registration, error) {
	operation := a.manager().Register
	if input.Preview {
		operation = a.manager().PreviewRegister
	}
	item, err := operation(ctx, input.Name, input.Path)
	return workspaceRegistration(item), err
}

func (a *workspaceRuntime) Clone(ctx context.Context, input workspaceaction.CloneInput) (workspaceaction.Registration, error) {
	root, err := a.creationRoot(input.Name, input.Path)
	if err != nil {
		return workspaceaction.Registration{}, err
	}
	operation := a.manager().Clone
	if input.Preview {
		operation = a.manager().PreviewClone
	}
	item, err := operation(ctx, input.Source, input.Name, root)
	return workspaceRegistration(item), err
}

func (w *workspaceRuntime) creationRoot(name, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if err := config.ValidateWorkspaceName(name); err != nil {
		return "", fmt.Errorf("workspace name: %w", err)
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

func (a *workspaceRuntime) List(ctx context.Context, limit int, cursor string) (workspaceaction.ListPage, error) {
	offset := 0
	if cursor != "" {
		value, err := strconv.Atoi(cursor)
		if err != nil || value < 0 {
			return workspaceaction.ListPage{}, fmt.Errorf("workspace cursor must be a non-negative integer")
		}
		offset = value
	}
	page, err := a.manager().List(ctx, limit, offset)
	if err != nil {
		return workspaceaction.ListPage{}, err
	}
	items := make([]workspaceaction.ListedWorkspace, len(page.Items))
	for index, item := range page.Items {
		items[index] = workspaceaction.ListedWorkspace{Workspace: workspaceIdentity(item), Default: item.Default, Available: item.Available, ManifestValid: item.ManifestValid}
	}
	return workspaceaction.ListPage{Returned: page.Returned, Total: page.Total, Limit: page.Limit, NextCursor: page.NextCursor, Items: items}, nil
}

func (a *workspaceRuntime) Status(ctx context.Context, input workspaceaction.StatusInput) (workspaceaction.Workspace, workspaceaction.StatusInventory, error) {
	resolved, err := a.resolve(ctx, input.Workspace)
	if err != nil {
		return workspaceaction.Workspace{}, workspaceaction.StatusInventory{}, err
	}
	page, err := artifact.Inventory(ctx, resolved.Root, artifact.InventoryOptions{Limit: input.Limit, Cursor: input.Cursor})
	if err != nil {
		return workspaceaction.Workspace{}, workspaceaction.StatusInventory{}, err
	}
	items := make([]workspaceaction.StatusArtifact, len(page.Items))
	for index, item := range page.Items {
		items[index] = statusArtifact(item)
	}
	return workspaceIdentity(resolved), workspaceaction.StatusInventory{Returned: page.Returned, Total: page.Total, Limit: page.Limit, NextCursor: page.NextCursor, ScanComplete: page.ScanComplete, Clean: page.Clean, Dirty: page.Dirty, Missing: page.Missing, Invalid: page.Invalid, Items: items, Warnings: append([]string(nil), page.Warnings...)}, nil
}

func (a *workspaceRuntime) Move(ctx context.Context, input workspaceaction.MoveInput) (workspaceaction.MoveArtifact, error) {
	source, err := a.resolve(ctx, input.SourceWorkspace)
	if err != nil {
		return workspaceaction.MoveArtifact{}, err
	}
	destination, err := a.resolve(ctx, input.DestinationWorkspace)
	if err != nil {
		return workspaceaction.MoveArtifact{}, err
	}
	operation := artifact.Move
	if input.Preview {
		operation = artifact.PreviewMove
	}
	item, err := operation(ctx, artifact.MoveRequest{SourceWorkspace: source.Root, DestinationWorkspace: destination.Root, Selector: artifact.Selector{Kind: input.Kind, LUID: input.LUID, Path: input.Path}})
	if err != nil {
		return moveArtifact(item), mapArtifactResolutionError("workspace.move", input.SourceWorkspace, input.LUID, err)
	}
	return moveArtifact(item), err
}

func (a *workspaceRuntime) SetDefault(ctx context.Context, input workspaceaction.SetDefaultInput) (workspaceaction.Workspace, error) {
	operation := a.manager().SetDefault
	if input.Preview {
		operation = a.manager().PreviewSetDefault
	}
	item, err := operation(ctx, input.Name)
	return workspaceIdentity(item), err
}

func (a *workspaceRuntime) Unregister(ctx context.Context, input workspaceaction.UnregisterInput) (workspaceaction.Workspace, error) {
	operation := a.manager().Unregister
	if input.Preview {
		operation = a.manager().PreviewUnregister
	}
	item, err := operation(ctx, input.Name)
	return workspaceIdentity(item), err
}

func (a *workspaceRuntime) ResolveWorkspace(ctx context.Context, name string) (workspaceaction.DeletionTarget, error) {
	item, err := a.manager().Resolve(ctx, name, "")
	if err != nil {
		return workspaceaction.DeletionTarget{}, err
	}
	page, err := artifact.Inventory(ctx, item.Root, artifact.InventoryOptions{Limit: 1})
	if err != nil {
		return workspaceaction.DeletionTarget{}, err
	}
	unmanaged, err := workspaceHasUnmanagedEntries(ctx, item.Root, page.ManagedPaths)
	if err != nil {
		return workspaceaction.DeletionTarget{}, err
	}
	return workspaceaction.DeletionTarget{Workspace: workspaceIdentity(item), Dirty: page.Dirty > 0 || page.Invalid > 0 || page.Missing > 0 || unmanaged, DirtyArtifacts: page.Dirty, InvalidArtifacts: page.Invalid + page.Missing}, nil
}
func (a *workspaceRuntime) DeleteWorkspace(ctx context.Context, request workspaceaction.WorkspaceDeleteRequest) error {
	current, err := a.ResolveWorkspace(ctx, request.Expected.Name)
	if err != nil {
		return err
	}
	if current.ID != request.Expected.ID || !sameWorkspaceRoot(current.Root, request.Expected.Root) {
		return errors.New("workspace identity changed during deletion revalidation")
	}
	if current.Dirty && !request.Force {
		return errors.New("workspace became dirty during deletion revalidation")
	}
	_, err = a.manager().Delete(ctx, workspacecore.Record{Name: current.Name, ID: current.ID, Root: current.Root})
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

func (a *workspaceRuntime) Clean(ctx context.Context, request workspaceaction.CleanInput) (workspaceaction.CleanResult, error) {
	resolved, err := a.resolve(ctx, request.Workspace)
	if err != nil {
		return workspaceaction.CleanResult{}, err
	}
	operation := workspacecore.Clean
	if request.Preview {
		operation = workspacecore.PreviewClean
	}
	result, err := operation(ctx, resolved.Root, request.Class)
	return workspaceaction.CleanResult{EntriesRemoved: result.EntriesRemoved, BytesRemoved: result.BytesRemoved, Removed: result.Removed}, err
}

func (a *workspaceRuntime) ResolveArtifact(ctx context.Context, input workspaceaction.DeleteArtifactInput) (workspaceaction.ArtifactTarget, error) {
	workspace, err := a.resolve(ctx, input.Workspace)
	if err != nil {
		return workspaceaction.ArtifactTarget{}, err
	}
	item, err := artifact.Resolve(ctx, workspace.Root, artifact.Selector{Kind: input.Kind, LUID: input.LUID, Path: input.Path})
	if err != nil {
		return workspaceaction.ArtifactTarget{}, mapArtifactResolutionError("workspace.artifact.delete", workspace.Name, input.LUID, err)
	}
	return deleteArtifact(item), err
}

func mapArtifactResolutionError(operation, workspace, resource string, cause error) error {
	ambiguous, ok := errors.AsType[*artifact.AmbiguousSelectorError](cause)
	if !ok {
		return cause
	}
	ambiguous.FullStatusCommand = commandhint.Command("workspace", "status", "--workspace", workspace, "--full")
	structured := &errs.Error{
		ID:               operation + ".ambiguous",
		Kind:             errs.KindUsage,
		Operation:        operation,
		Resource:         resource,
		Summary:          "The managed artifact selector matched more than one exact artifact.",
		Cause:            cause,
		Retryable:        errs.Bool(false),
		CorrectiveAction: "Review the bounded candidates with " + ambiguous.FullStatusCommand + ", then select one exact workspace-relative artifact path.",
		Phase:            errs.PhaseValidation,
		Outcome:          errs.OutcomeNotAttempted,
	}
	return clierr.WithOutput(ambiguousArtifactOutput{
		Status:            "ambiguous",
		Workspace:         workspace,
		Kind:              ambiguous.Kind,
		Name:              ambiguous.Name,
		LUID:              ambiguous.LUID,
		Candidates:        append([]artifact.AmbiguousCandidate(nil), ambiguous.Candidates...),
		Truncated:         ambiguous.Truncated,
		FullStatusCommand: ambiguous.FullStatusCommand,
	}, structured)
}

type ambiguousArtifactOutput struct {
	Status            string                        `json:"status"`
	Workspace         string                        `json:"workspace"`
	Kind              string                        `json:"kind"`
	Name              string                        `json:"name,omitempty"`
	LUID              string                        `json:"luid,omitempty"`
	Candidates        []artifact.AmbiguousCandidate `json:"candidates"`
	Truncated         int                           `json:"truncated"`
	FullStatusCommand string                        `json:"full_status_command"`
}

func (a *workspaceRuntime) DeleteArtifact(ctx context.Context, request workspaceaction.ArtifactDeleteRequest) (workspaceaction.ArtifactTarget, error) {
	workspace, err := a.resolve(ctx, request.Workspace)
	if err != nil {
		return workspaceaction.ArtifactTarget{}, err
	}
	item, err := artifact.Delete(ctx, artifact.DeleteRequest{Workspace: workspace.Root, Expected: artifact.Item{Kind: request.Expected.Kind, LUID: request.Expected.LUID, Name: request.Expected.Name, Path: request.Expected.Path, CanonicalPath: request.Expected.CanonicalPath, State: request.Expected.State, ServerOrigin: request.Expected.ServerOrigin, SiteLUID: request.Expected.SiteLUID, BaselineFingerprint: request.Expected.BaselineFingerprint, CurrentFingerprint: request.Expected.CurrentFingerprint, TreeFingerprint: request.Expected.TreeFingerprint}})
	return deleteArtifact(item), err
}

func statusArtifact(item artifact.Item) workspaceaction.StatusArtifact {
	diagnostic := ""
	if len(item.Warnings) > 0 {
		diagnostic = item.Warnings[0]
	}
	return workspaceaction.StatusArtifact{Kind: item.Kind, LUID: item.LUID, Name: item.Name, Path: item.Path, State: item.State, Reason: item.Reason, Diagnostic: diagnostic, CanonicalPath: item.CanonicalPath, BaselineFingerprint: item.BaselineFingerprint, CurrentFingerprint: item.CurrentFingerprint}
}
func moveArtifact(item artifact.Item) workspaceaction.MoveArtifact {
	return workspaceaction.MoveArtifact{Kind: item.Kind, LUID: item.LUID, Name: item.Name, Path: item.Path, State: item.State, ServerOrigin: item.ServerOrigin, SiteLUID: item.SiteLUID, BaselineFingerprint: item.BaselineFingerprint, CurrentFingerprint: item.CurrentFingerprint, Warnings: append([]string(nil), item.Warnings...)}
}
func deleteArtifact(item artifact.Item) workspaceaction.ArtifactTarget {
	return workspaceaction.ArtifactTarget{Kind: item.Kind, LUID: item.LUID, Name: item.Name, Path: item.Path, State: item.State, CanonicalPath: item.CanonicalPath, ServerOrigin: item.ServerOrigin, SiteLUID: item.SiteLUID, BaselineFingerprint: item.BaselineFingerprint, CurrentFingerprint: item.CurrentFingerprint, TreeFingerprint: item.TreeFingerprint, Warnings: append([]string(nil), item.Warnings...)}
}

func workspaceIdentity(item workspacecore.Record) workspaceaction.Workspace {
	return workspaceaction.Workspace{Name: item.Name, ID: item.ID, Root: item.Root, Status: item.Status, Violations: item.Violations}
}

func workspaceRegistration(item workspacecore.Record) workspaceaction.Registration {
	return workspaceaction.Registration{Workspace: workspaceIdentity(item), ManifestVersion: 1, Registered: item.Available && item.ManifestValid}
}
