//go:build windows

package workspace

import (
	"strings"

	"golang.org/x/sys/windows"
)

// resolveExistingPath asks the filesystem for the path of the opened object.
// filepath.EvalSymlinks does not resolve Windows mount-point junctions, which
// Go reports as name-surrogate reparse points rather than symbolic links.
func resolveExistingPath(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 512)
	for {
		size, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
		if err != nil {
			return "", err
		}
		if size >= uint32(len(buffer)) {
			buffer = make([]uint16, size+1)
			continue
		}
		resolved := windows.UTF16ToString(buffer[:size])
		if unc, ok := strings.CutPrefix(resolved, `\\?\UNC\`); ok {
			return `\\` + unc, nil
		}
		if dos, ok := strings.CutPrefix(resolved, `\\?\`); ok && len(dos) >= 2 && dos[1] == ':' {
			return dos, nil
		}
		return resolved, nil
	}
}
