//go:build linux || darwin

package managedpolicy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func unixLocatorPath() string {
	if runtime.GOOS == "darwin" {
		// /etc is a system symlink on macOS. Use its fixed canonical spelling,
		// without resolving or trusting arbitrary caller-supplied symlinks.
		return "/private/etc/tadx-policy-location.json"
	}
	return "/etc/tadx-policy-location.json"
}

func SystemPath() (string, error) { path, _, err := systemPolicyLocation(); return path, err }

func systemPolicyLocation() (string, bool, error) {
	return readUnixLocation(unixLocatorPath(), defaultUnixPolicyPath())
}

func readUnixLocation(locator, defaultPath string) (string, bool, error) {
	data, _, err := secureRead(locator)
	if errors.Is(err, os.ErrNotExist) {
		return defaultPath, false, nil
	}
	if err != nil {
		return "", true, fmt.Errorf("cannot read protected policy locator: %w", err)
	}
	var dir string
	if err := json.Unmarshal(data, &dir); err != nil || validateUnixDirectory(dir) != nil {
		return "", true, errors.New("invalid protected policy locator; expected a JSON string containing a clean absolute directory; run tadx policy install to repair it")
	}
	return filepath.Join(dir, "managed-policy.json"), true, nil
}
