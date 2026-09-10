package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/config"
	"gopkg.in/yaml.v3"
)

// RecoveryError carries local prerequisite guidance across action boundaries.
type RecoveryError struct{ summary, action, resource string }

func (e *RecoveryError) Error() string                { return e.summary }
func (e *RecoveryError) Retryable() bool              { return false }
func (e *RecoveryError) CorrectiveAction() string     { return e.action }
func (e *RecoveryError) PrerequisiteKind() string     { return "workspace" }
func (e *RecoveryError) PrerequisiteResource() string { return e.resource }
func (e *RecoveryError) PrerequisiteSummary() string  { return e.summary }
func recoveryError(summary, action string, resource ...string) error {
	err := &RecoveryError{summary: summary, action: action}
	if len(resource) > 0 {
		err.resource = resource[0]
	}
	return err
}

func emptyDirectory(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	names, err := file.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return len(names) == 0, nil
	}
	return false, err
}

func replacementGuidance(configuration config.Config, removed, environment string) string {
	var names []string
	for name, registration := range configuration.Workspaces {
		if strings.EqualFold(name, removed) {
			continue
		}
		if record := recordFromRegistration(name, registration, configuration.DefaultWorkspace); record.Available && record.ManifestValid {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		command := commandhint.Command("workspace", "set-default", "<name>")
		if environment != "" {
			command = commandhint.Command("env", "update", environment, "--default-workspace", "<name>")
		}
		return "No available replacement workspace. Restore an existing workspace or run tadx workspace create <name>, then " + command + "."
	}
	command := commandhint.Command("workspace", "set-default", names[0])
	if environment != "" {
		command = commandhint.Command("env", "update", environment, "--default-workspace", names[0])
	}
	more := ""
	if len(names) > 5 {
		names = names[:5]
		more = "; more: tadx workspace list"
	}
	return "Select a replacement before removal, for example: " + command + ". Available workspaces: " + strings.Join(names, ", ") + more + "."
}

// createRoot initializes only a missing or truly empty real directory. The
// returned finalizer removes only entries created here on rollback and preserves
// a pre-existing root, along with any files another process subsequently adds.
func createRoot(root string, manifest Manifest) (finalize func(bool) error, resultErr error) {
	if filepath.Dir(root) == root {
		return nil, errors.New("workspace creation target must not be a filesystem root")
	}
	info, err := os.Lstat(root)
	existed := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if existed {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, recoveryError("workspace root must be a real directory", "Choose a new path or an empty real directory for tadx workspace create.")
		}
		empty, err := emptyDirectory(root)
		if err != nil {
			return nil, err
		}
		if !empty {
			if existing, err := ReadManifest(root); err == nil {
				return nil, recoveryError("workspace root already contains a managed workspace", "Inspect registered workspaces with tadx workspace list. If this workspace is not registered, run: "+commandhint.Command("workspace", "register", existing.Workspace.Name, "--path", root))
			}
			return nil, recoveryError("workspace root is a nonempty directory without a valid workspace manifest", "Keep the existing files and choose a new or empty directory: "+commandhint.Command("workspace", "create", manifest.Workspace.Name, "--path", "<new-or-empty-directory>"))
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(root), 0o700); err != nil {
			return nil, err
		}
		if err := os.Mkdir(root, 0o700); err != nil {
			return nil, err
		}
		info, err = os.Lstat(root)
		if err != nil {
			return nil, err
		}
	}
	directory, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	opened, err := directory.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		_ = directory.Close()
		return nil, errors.New("workspace root changed during creation")
	}
	type createdEntry struct {
		name string
		info os.FileInfo
		data []byte
	}
	var created []createdEntry
	finish := func(rollback bool) error {
		var failures []error
		if rollback {
			for index := len(created) - 1; index >= 0; index-- {
				entry := created[index]
				current, err := directory.Lstat(entry.name)
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				if err != nil || !os.SameFile(entry.info, current) {
					failures = append(failures, fmt.Errorf("creation rollback preserved changed entry %q", entry.name))
					continue
				}
				if entry.data != nil {
					if current.Size() != int64(len(entry.data)) {
						failures = append(failures, fmt.Errorf("creation rollback preserved changed entry %q", entry.name))
						continue
					}
					data, err := directory.ReadFile(entry.name)
					if err != nil || !bytes.Equal(data, entry.data) {
						failures = append(failures, fmt.Errorf("creation rollback preserved changed entry %q", entry.name))
						continue
					}
				}
				if err := directory.Remove(entry.name); err != nil {
					failures = append(failures, fmt.Errorf("creation rollback preserved nonempty or unavailable entry %q: %w", entry.name, err))
				}
			}
		}
		failures = append(failures, directory.Close())
		if rollback && !existed {
			if current, err := os.Lstat(root); err == nil && os.SameFile(info, current) {
				failures = append(failures, os.Remove(root))
			}
		}
		return errors.Join(failures...)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, finish(true))
		}
	}()
	for _, name := range []string{"artifacts", ".tadx"} {
		if err := directory.Mkdir(name, 0o700); err != nil {
			return nil, err
		}
		info, err := directory.Lstat(name)
		if err != nil {
			return nil, err
		}
		created = append(created, createdEntry{name: name, info: info})
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	file, err := directory.OpenFile(config.WorkspaceConfigName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	fileInfo, statErr := file.Stat()
	written, writeErr := file.Write(data)
	closeErr := file.Close()
	if statErr == nil {
		created = append(created, createdEntry{name: config.WorkspaceConfigName, info: fileInfo, data: data[:written]})
	}
	if err := errors.Join(statErr, writeErr, closeErr); err != nil {
		return nil, err
	}
	return finish, nil
}
