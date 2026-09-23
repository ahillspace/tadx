package managedpolicy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func defaultSystemPath() (string, error) {
	// The native 64-bit location is stable across 32-bit and 64-bit clients.
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFilesX64, 0)
	if err != nil {
		base, err = windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	}
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(base) || strings.HasPrefix(base, `\\`) {
		return "", errors.New("system policy location must use a local absolute path")
	}
	return filepath.Join(base, "TADX", "managed-policy.json"), nil
}

func secureRead(path string) ([]byte, []ProtectionCheck, error) {
	checks := []ProtectionCheck{}
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) {
		return nil, checks, errors.New("managed policy requires a local absolute path")
	}
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	current := volume + `\`
	parts := strings.Split(strings.TrimPrefix(clean, current), `\`)
	paths := []string{current}
	for _, part := range parts {
		current = filepath.Join(current, part)
		paths = append(paths, current)
	}
	handles := make([]windows.Handle, 0, len(paths))
	var protectionErr error
	defer func() {
		for _, handle := range handles {
			windows.CloseHandle(handle)
		}
	}()
	for index, item := range paths {
		file := index == len(paths)-1
		name, err := windows.UTF16PtrFromString(item)
		if err != nil {
			return nil, checks, errors.New("invalid policy path")
		}
		protected := file || index == len(paths)-2
		access := uint32(windows.FILE_READ_ATTRIBUTES)
		if protected {
			access |= windows.READ_CONTROL
		}
		share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE)
		if file {
			access |= windows.FILE_READ_DATA
			share = windows.FILE_SHARE_READ
		}
		handle, err := windows.CreateFile(name, access, share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if err != nil {
			if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
				return nil, checks, os.ErrNotExist
			}
			checks = append(checks, ProtectionCheck{Path: item, Kind: "open", Reason: "cannot open policy path for security inspection"})
			return nil, checks, errors.New("cannot open managed policy path for security inspection")
		}
		handles = append(handles, handle)
		var info windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
			return nil, checks, errors.New("cannot inspect managed policy path")
		}
		checkErr := error(nil)
		if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			checkErr = errors.New("reparse points are not permitted")
		} else if file && (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 || info.NumberOfLinks != 1) {
			checkErr = errors.New("policy must be a regular file with one link")
		} else if !file && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			checkErr = errors.New("policy ancestor must be a directory")
		}
		if checkErr == nil && protected {
			sd, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
			if err != nil {
				checkErr = errors.New("cannot read owner and DACL")
			} else {
				checkErr = checkSecurityDescriptor(sd, file, index == len(paths)-2)
			}
		}
		kind := "path-integrity"
		if protected {
			kind = "owner-acl-and-links"
		}
		check := ProtectionCheck{Path: item, Kind: kind, Passed: checkErr == nil}
		if checkErr != nil {
			check.Reason = checkErr.Error()
		}
		checks = append(checks, check)
		if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return nil, checks, errors.New("reparse points are not permitted")
		}
		if !file && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			return nil, checks, errors.New("policy ancestor must be a directory")
		}
		if protectionErr == nil {
			protectionErr = checkErr
		}
		if file {
			if protectionErr != nil {
				return nil, checks, fmt.Errorf("insecure managed policy path: %s", protectionErr)
			}
			// Transfer the protected handle to os.File while ancestors stay locked.
			handles = handles[:len(handles)-1]
			opened := os.NewFile(uintptr(handle), item)
			defer opened.Close()
			data, err := readBounded(opened)
			return data, checks, err
		}
	}
	return nil, checks, errors.New("invalid policy path")
}

func trustedSID(sid *windows.SID) bool {
	if sid == nil {
		return false
	}
	switch sid.String() {
	case "S-1-5-18", "S-1-5-32-544", "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464":
		return true // LocalSystem, built-in Administrators, and TrustedInstaller.
	default:
		return false
	}
}

func checkSecurityDescriptor(sd *windows.SECURITY_DESCRIPTOR, file, immediateParent bool) error {
	owner, _, err := sd.Owner()
	if err != nil || !trustedSID(owner) {
		return errors.New("owner must be SYSTEM, Administrators, or TrustedInstaller")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return errors.New("a non-null DACL is required")
	}
	// Ancestors can permit creation of unrelated children. They must not permit
	// deleting/replacing the protected child or changing their own protection.
	const fileDeleteChild = 0x40
	unsafeMask := uint32(windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER | windows.GENERIC_ALL | windows.GENERIC_WRITE | windows.FILE_WRITE_ATTRIBUTES | windows.FILE_WRITE_EA | fileDeleteChild)
	if file || immediateParent {
		unsafeMask |= windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA
	}
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return errors.New("cannot inspect DACL entries")
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		// Unknown/callback/object ACEs fail closed instead of guessing semantics.
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("unsupported effective DACL entry")
		}
		if uint32(ace.Mask)&unsafeMask == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !trustedSID(sid) {
			return errors.New("DACL grants modification rights to a non-administrator principal")
		}
	}
	return nil
}
