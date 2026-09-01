package artifact

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/ahillspace/tadx/internal/lock"
)

// workspaceLockName is the advisory lock file every workspace mutation holds.
// It lives at the workspace root and serializes the find/backup/stage/rename
// critical section against other tadx processes mutating the same tree. The
// modular-monolith boundary (internal/architecture) grants the artifact manager
// sole permission to import internal/lock precisely because it owns this
// section; dropping the lock silently would weaken a stated safety invariant.
const workspaceLockName = ".tadx.lock"

// lockWorkspace acquires the advisory workspace lock for a single root. Callers
// must Release the returned handle when the mutation completes.
func lockWorkspace(workspace string) (*lock.Handle, error) {
	handle, err := lock.Acquire(filepath.Join(workspace, workspaceLockName))
	if err != nil {
		return nil, fmt.Errorf("acquire workspace lock: %w", err)
	}
	return handle, nil
}

// lockWorkspaces acquires the advisory lock for one or more workspace roots in a
// deterministic (sorted) order so concurrent cross-workspace moves cannot
// deadlock. Duplicate roots are locked once. The returned function releases
// every acquired handle in reverse order and must always be called.
func lockWorkspaces(workspaces ...string) (func(), error) {
	unique := make([]string, 0, len(workspaces))
	seen := make(map[string]struct{}, len(workspaces))
	for _, workspace := range workspaces {
		key := filepath.Clean(workspace)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}
	sort.Strings(unique)

	handles := make([]*lock.Handle, 0, len(unique))
	release := func() {
		for index := len(handles) - 1; index >= 0; index-- {
			_ = handles[index].Release()
		}
	}
	for _, workspace := range unique {
		handle, err := lockWorkspace(workspace)
		if err != nil {
			release()
			return nil, err
		}
		handles = append(handles, handle)
	}
	return release, nil
}
