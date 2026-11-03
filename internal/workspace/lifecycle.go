package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/lock"
)

// ArtifactLockName is the advisory file lock shared with earlier CLI versions.
const ArtifactLockName = ".tadx.lock"

const lifecycleLockPrefix = ".tadx-workspace-"

// Mutation holds a workspace's lifecycle lease and exclusive artifact lock.
// Root deletion additionally holds the lifecycle lock exclusively. The lifecycle
// lock lives outside the workspace, so Windows can rename the root after its
// interior lock handles close without admitting another operation.
type Mutation struct {
	*workspaceUse
	artifact *lock.Handle
}

type workspaceUse struct {
	root        string
	identity    os.FileInfo
	manifest    [sha256.Size]byte
	hasManifest bool
	lifecycle   *lock.Handle
	exclusive   bool
}

// ErrWorkspaceInUse is returned when deletion cannot own an active workspace.
var ErrWorkspaceInUse error = workspaceInUseError{}

type workspaceInUseError struct{}

func (workspaceInUseError) Error() string   { return "workspace is in use by another operation" }
func (workspaceInUseError) Retryable() bool { return true }
func (workspaceInUseError) CorrectiveAction() string {
	return "Retry workspace deletion after active artifact operations and downloads finish."
}

// BeginMutation acquires the lifecycle lease before the artifact lock, then
// checks the root identity again after any wait. Artifact recovery must run
// under this ownership before inspecting or changing managed paths.
func BeginMutation(ctx context.Context, root string) (*Mutation, error) {
	use, err := beginWorkspaceUse(ctx, root, false)
	if err != nil {
		return nil, err
	}
	held, err := lock.AcquireContext(ctx, filepath.Join(use.root, ArtifactLockName))
	if err != nil {
		_ = use.release()
		return nil, fmt.Errorf("acquire workspace artifact lock: %w", err)
	}
	mutation := &Mutation{workspaceUse: use, artifact: held}
	if err := mutation.Validate(); err != nil {
		_ = mutation.Release()
		return nil, err
	}
	return mutation, nil
}

// Root returns the canonical root protected by this live ownership token.
func (m *Mutation) Root() string {
	if m == nil || m.workspaceUse == nil {
		return ""
	}
	return m.root
}

// Validate rejects a released token or a root replaced while ownership waited.
func (m *Mutation) Validate() error {
	if m == nil || m.workspaceUse == nil || (m.artifact == nil && !m.exclusive) {
		return errors.New("workspace mutation ownership is not held")
	}
	return m.workspaceUse.validate()
}

// Release drops interior ownership before releasing the lifecycle lease.
func (m *Mutation) Release() error {
	if m == nil {
		return nil
	}
	err := m.releaseArtifact()
	if m.workspaceUse != nil {
		err = errors.Join(err, m.workspaceUse.release())
	}
	return err
}

func (m *Mutation) releaseArtifact() error {
	err := m.artifact.Release()
	m.artifact = nil
	return err
}

func beginWorkspaceUse(ctx context.Context, root string, exclusive bool) (*workspaceUse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(canonical)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("workspace root must be an available real directory")
	}
	manifest, hasManifest, err := workspaceManifestIdentity(canonical)
	if err != nil {
		return nil, err
	}
	path := lifecycleLockPath(canonical)
	var held *lock.Handle
	if exclusive {
		held, err = lock.TryAcquire(path)
	} else {
		held, err = lock.AcquireSharedContext(ctx, path)
	}
	if errors.Is(err, lock.ErrLocked) {
		return nil, ErrWorkspaceInUse
	}
	if err != nil {
		return nil, fmt.Errorf("acquire workspace lifecycle lock; its parent directory must permit coordination: %w", errs.WithWorkspaceRelativePaths(err, filepath.Dir(canonical)))
	}
	use := &workspaceUse{root: canonical, identity: info, manifest: manifest, hasManifest: hasManifest, lifecycle: held, exclusive: exclusive}
	if err := use.validate(); err != nil {
		_ = use.release()
		return nil, err
	}
	return use, nil
}

func (u *workspaceUse) validate() error {
	if u.lifecycle == nil {
		return errors.New("workspace lifecycle ownership is not held")
	}
	current, err := os.Lstat(u.root)
	if err != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(current, u.identity) {
		return errors.New("workspace root was removed or replaced while the operation waited")
	}
	manifest, exists, err := workspaceManifestIdentity(u.root)
	if err != nil || exists != u.hasManifest || manifest != u.manifest {
		return errors.New("workspace manifest identity changed while the operation waited")
	}
	return nil
}

func workspaceManifestIdentity(root string) ([sha256.Size]byte, bool, error) {
	data, _, err := readBoundedManifest(filepath.Join(root, "tadx.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return [sha256.Size]byte{}, false, nil
	}
	if err != nil {
		return [sha256.Size]byte{}, false, err
	}
	return sha256.Sum256(data), true, nil
}

func (u *workspaceUse) release() error {
	err := u.lifecycle.Release()
	u.lifecycle = nil
	return err
}

func lifecycleLockPath(root string) string {
	// Every user and invocation reaching this canonical root must lock the
	// same sibling inode. User-cache paths would split ownership when a caller
	// changes environment or account. Keep the file after release so waiters
	// never acquire an unlinked, superseded lock inode.
	directory := filepath.Dir(root)
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	key := sha256.Sum256([]byte(root))
	return filepath.Join(directory, fmt.Sprintf("%s%x.lock", lifecycleLockPrefix, key))
}

// IsLifecycleControlFile identifies empty coordination files left by nested
// workspaces. Nonempty or linked lookalikes contain unowned data and do not
// qualify as disposable workspace control state.
func IsLifecycleControlFile(entry os.DirEntry) (bool, error) {
	key, ok := strings.CutPrefix(entry.Name(), lifecycleLockPrefix)
	if !ok || !entry.Type().IsRegular() {
		return false, nil
	}
	key, ok = strings.CutSuffix(key, ".lock")
	if !ok || len(key) != 2*sha256.Size {
		return false, nil
	}
	if _, err := hex.DecodeString(key); err != nil {
		return false, nil
	}
	info, err := entry.Info()
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular() && info.Size() == 0, nil
}

// acquireDeletion refuses active owners instead of waiting while retaining a
// registry lock. Lock order is lifecycle, artifact, downloads, configuration.
// The legacy interior locks also cover commands from versions predating leases.
func acquireDeletion(ctx context.Context, root string) (*Mutation, *lock.Handle, error) {
	use, err := beginWorkspaceUse(ctx, root, true)
	if err != nil {
		return nil, nil, err
	}
	mutation := &Mutation{workspaceUse: use}
	fail := func(err error) (*Mutation, *lock.Handle, error) {
		_ = mutation.Release()
		if errors.Is(err, lock.ErrLocked) {
			err = ErrWorkspaceInUse
		}
		return nil, nil, err
	}
	artifactPath := filepath.Join(use.root, ArtifactLockName)
	artifactInfo, statErr := os.Lstat(artifactPath)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fail(statErr)
	}
	if artifactInfo == nil || !artifactInfo.IsDir() || artifactInfo.Mode()&os.ModeSymlink != 0 {
		mutation.artifact, err = lock.TryAcquire(artifactPath)
		if err != nil {
			return fail(err)
		}
	} else {
		// A real directory at the legacy file-lock path cannot be opened by
		// its O_RDWR lock protocol, so it cannot have an active legacy owner.
		// Exclusive lifecycle ownership excludes new commands while the
		// caller applies its ordinary dirty/force policy to these user files.
	}
	control, err := controlDirectory(use.root, false)
	if errors.Is(err, os.ErrNotExist) {
		return mutation, nil, nil
	}
	if err != nil {
		return fail(err)
	}
	downloads, err := lock.TryAcquire(filepath.Join(control, downloadLockName))
	if err != nil {
		return fail(err)
	}
	return mutation, downloads, nil
}
