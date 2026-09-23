package managedpolicy

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"github.com/ahillspace/tadx/internal/capability"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func Install(ctx context.Context, options InstallOptions, definitions []capability.Definition) (InstallResult, error) {
	return runWindowsInstall(ctx, options, definitions, windows.GetCurrentProcessToken().IsElevated(), elevateInstall)
}

func runWindowsInstall(ctx context.Context, options InstallOptions, definitions []capability.Definition, elevated bool, elevate func(context.Context, InstallResult) (InstallResult, error)) (InstallResult, error) {
	out, data, err := prepareInstall(options, definitions)
	if err != nil {
		return out, err
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if !elevated {
		return elevate(ctx, out)
	}
	return installWith(ctx, out, data, &windowsInstaller{root: registry.LOCAL_MACHINE, key: locatorKey, definitions: definitions})
}

func prepareInstall(options InstallOptions, definitions []capability.Definition) (InstallResult, []byte, error) {
	out := InstallResult{Template: options.Template, Phase: "validation"}
	if out.Template == "" || out.Template == "admin" {
		out.Template = "superuser"
	}
	doc, err := Template(out.Template, definitions)
	if err != nil {
		return out, nil, err
	}
	dir := options.Directory
	if dir == "" {
		path, err := defaultSystemPath()
		if err != nil {
			return out, nil, err
		}
		dir = filepath.Dir(path)
	}
	if err := validateLocalDirectory(dir); err != nil {
		return out, nil, err
	}
	if err := dedicatedDirectory(dir); err != nil {
		return out, nil, err
	}
	out.Path = filepath.Join(dir, "managed-policy.json")
	data, err := json.MarshalIndent(doc, "", "  ")
	return out, append(data, '\n'), err
}

func dedicatedDirectory(dir string) error {
	if sameInstallPath(dir, filepath.VolumeName(dir)+`\`) {
		return errors.New("the drive root is not a dedicated policy directory")
	}
	profiles, err := windows.KnownFolderPath(windows.FOLDERID_UserProfiles, 0)
	if err == nil && (sameInstallPath(dir, profiles) || sameInstallPath(filepath.Dir(dir), profiles)) {
		return errors.New("a user home or the profiles directory is not a dedicated policy directory")
	}
	for _, id := range []*windows.KNOWNFOLDERID{windows.FOLDERID_Profile, windows.FOLDERID_ProgramFiles, windows.FOLDERID_ProgramFilesX64, windows.FOLDERID_ProgramFilesX86, windows.FOLDERID_ProgramData, windows.FOLDERID_Windows, windows.FOLDERID_Public} {
		path, err := windows.KnownFolderPath(id, 0)
		if err == nil && sameInstallPath(path, dir) {
			return errors.New("choose a dedicated policy directory, not a shared system directory or user home")
		}
	}
	return nil
}

type windowsInstaller struct {
	root        registry.Key
	key         string
	definitions []capability.Definition
}

func (w *windowsInstaller) current() (string, error) {
	path, _, err := readLocation(w.root, w.key)
	return path, err
}
func (w *windowsInstaller) publish(dir string) (bool, error) {
	return publishLocation(w.root, w.key, dir)
}
func (w *windowsInstaller) verify(path string) error {
	policy := loadPath(path, w.definitions)
	if policy.Status().State != StateActive {
		return fmt.Errorf("installed policy verification failed: %s", policy.Status().Reason)
	}
	return nil
}

func (w *windowsInstaller) lock(ctx context.Context) (func(), error) {
	runtime.LockOSThread()
	sd, err := windows.SecurityDescriptorFromString("O:BAG:BAD:P(A;;GA;;;SY)(A;;GA;;;BA)")
	if err != nil {
		runtime.UnlockOSThread()
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	name := windows.StringToUTF16Ptr(`Global\TADX.ManagedPolicy.Install`)
	handle, err := windows.CreateMutex(&sa, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		runtime.UnlockOSThread()
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			windows.CloseHandle(handle)
			runtime.UnlockOSThread()
			return nil, err
		}
		state, err := windows.WaitForSingleObject(handle, 100)
		if err != nil {
			windows.CloseHandle(handle)
			runtime.UnlockOSThread()
			return nil, err
		}
		if state == windows.WAIT_OBJECT_0 || state == windows.WAIT_ABANDONED {
			return func() { windows.ReleaseMutex(handle); windows.CloseHandle(handle); runtime.UnlockOSThread() }, nil
		}
		if state != uint32(windows.WAIT_TIMEOUT) {
			windows.CloseHandle(handle)
			runtime.UnlockOSThread()
			return nil, errors.New("cannot acquire policy installation lock")
		}
	}
}

// Keep every ancestor open without delete sharing throughout installation.
// Only the selected leaf directory is secured; ancestor ACLs are not inspected.
func (w *windowsInstaller) prepare(dir string) (func(), bool, error) {
	var handles []windows.Handle
	closePaths := func() {
		for i := len(handles) - 1; i >= 0; i-- {
			windows.CloseHandle(handles[i])
		}
	}
	if err := validateLocalDirectory(dir); err != nil {
		return closePaths, false, err
	}
	if err := dedicatedDirectory(dir); err != nil {
		return closePaths, false, err
	}
	current := filepath.VolumeName(dir) + `\`
	paths := []string{current}
	for _, part := range strings.Split(strings.TrimPrefix(dir, current), `\`) {
		current = filepath.Join(current, part)
		paths = append(paths, current)
	}
	changed := false
	for i, path := range paths {
		leaf := i == len(paths)-1
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return closePaths, changed, err
		}
		access := uint32(windows.FILE_READ_ATTRIBUTES)
		if leaf {
			access |= windows.READ_CONTROL | windows.WRITE_DAC | windows.WRITE_OWNER | windows.FILE_LIST_DIRECTORY
		}
		handle, err := windows.CreateFile(name, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) && leaf {
			sd, sdErr := windows.SecurityDescriptorFromString(protectedSDDL)
			if sdErr != nil {
				return closePaths, changed, sdErr
			}
			sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
			if err := windows.CreateDirectory(name, &sa); err != nil {
				return closePaths, changed, err
			}
			changed = true
			handle, err = windows.CreateFile(name, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		}
		if err != nil {
			return closePaths, changed, fmt.Errorf("cannot open policy directory (its parent must already exist): %w", err)
		}
		handles = append(handles, handle)
		var info windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
			return closePaths, changed, err
		}
		if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			return closePaths, changed, errors.New("policy installation path must contain only ordinary directories, without reparse points")
		}
		if leaf {
			if err := onlyPolicyContents(path); err != nil {
				return closePaths, changed, err
			}
			if err := protectHandle(handle, windows.SE_FILE_OBJECT, protectedSDDL); err != nil {
				return closePaths, changed, err
			}
			changed = true
			// Check again after removing ordinary-user write access.
			if err := onlyPolicyContents(path); err != nil {
				return closePaths, changed, err
			}
			sd, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
			if err != nil {
				return closePaths, changed, err
			}
			if err := checkSecurityDescriptor(sd, false, true); err != nil {
				return closePaths, changed, err
			}
		}
	}
	return closePaths, changed, nil
}

func onlyPolicyContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.EqualFold(entry.Name(), "managed-policy.json") {
			return errors.New("policy installation requires a dedicated directory containing no unrelated files or directories")
		}
		path := filepath.Join(dir, entry.Name())
		handle, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if err != nil {
			return err
		}
		var info windows.ByHandleFileInformation
		err = windows.GetFileInformationByHandle(handle, &info)
		windows.CloseHandle(handle)
		if err != nil {
			return err
		}
		if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || info.NumberOfLinks != 1 {
			return errors.New("existing policy must be an ordinary file with one link")
		}
	}
	return nil
}

func (w *windowsInstaller) write(path string, data []byte) error {
	if err := onlyPolicyContents(filepath.Dir(path)); err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(protectedSDDL)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	temp := filepath.Join(filepath.Dir(path), ".managed-policy-"+rand.Text()+".tmp")
	handle, err := windows.CreateFile(windows.StringToUTF16Ptr(temp), windows.GENERIC_WRITE, 0, &sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	file := os.NewFile(uintptr(handle), temp)
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	return windows.MoveFileEx(windows.StringToUTF16Ptr(temp), windows.StringToUTF16Ptr(path), windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
