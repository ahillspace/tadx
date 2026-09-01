// Package pathspec provides OS-independent path predicates.
//
// tadx runs its quality gates on Linux and Windows, so any path check that must
// behave identically on every platform cannot rely on the runtime-specific
// standard-library helpers: filepath.IsAbs, filepath.VolumeName, and
// filepath.Rel all interpret separators and volumes according to the OS the
// binary was built for. A guard written with those helpers silently passes on
// one runner and fails on another. The predicates here recognize both Unix and
// Windows conventions regardless of the runtime OS so the same input yields the
// same verdict everywhere.
package pathspec

import (
	"path"
	"path/filepath"
	"strings"
)

// IsAbs reports whether p is absolute under either the Unix or the Windows
// convention, independent of the runtime OS. It recognizes a leading slash
// (Unix), a leading backslash or UNC prefix, and a drive-letter volume such as
// "C:" (Windows, covering both "C:\dir" and drive-relative "C:dir").
func IsAbs(p string) bool {
	if p == "" {
		return false
	}
	if p[0] == '/' || p[0] == '\\' {
		return true
	}
	if hasDriveLetter(p) {
		return true
	}
	// Cover any remaining native-absolute forms the explicit checks miss.
	return filepath.IsAbs(p)
}

// HasSeparator reports whether p contains either separator convention.
func HasSeparator(p string) bool {
	return strings.ContainsAny(p, `/\`)
}

// Escapes reports whether p, treated as a workspace-relative path, points
// outside its base. Absolute paths (either convention) are treated as escaping.
// Both separator conventions are normalized to forward slashes before the
// lexical ".." check so the verdict is identical on every OS.
func Escapes(p string) bool {
	if IsAbs(p) {
		return true
	}
	slashed := path.Clean(strings.ReplaceAll(p, `\`, "/"))
	return slashed == ".." || strings.HasPrefix(slashed, "../")
}

// hasDriveLetter reports whether p begins with a Windows drive volume ("C:").
func hasDriveLetter(p string) bool {
	if len(p) < 2 || p[1] != ':' {
		return false
	}
	c := p[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
