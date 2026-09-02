// Package architecture enforces TADX's modular-monolith import boundaries.
package architecture

import (
	"bufio"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Violation is one forbidden production import.
type Violation struct {
	File   string
	Import string
	Reason string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s imports %s: %s", v.File, v.Import, v.Reason)
}

// Check scans production Go files below root and reports forbidden imports.
func Check(root string) ([]Violation, error) {
	modulePath, err := readModulePath(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}

	var violations []Violation
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && path != root && ignoredDirectory(entry.Name()) {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		imports, err := fileImports(path)
		if err != nil {
			return fmt.Errorf("parse %s: %w", relative, err)
		}
		for _, imported := range imports {
			if reason := forbiddenReason(relative, imported, modulePath); reason != "" {
				violations = append(violations, Violation{File: relative, Import: imported, Reason: reason})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		if violations[i].Import != violations[j].Import {
			return violations[i].Import < violations[j].Import
		}
		return violations[i].Reason < violations[j].Reason
	})
	return violations, nil
}

func ignoredDirectory(name string) bool {
	return name == ".git" || name == "vendor" || name == "archived"
}

func fileImports(path string) ([]string, error) {
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	imports := make([]string, 0, len(parsed.Imports))
	for _, specification := range parsed.Imports {
		value, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			return nil, err
		}
		imports = append(imports, value)
	}
	sort.Strings(imports)
	return imports, nil
}

func forbiddenReason(file, imported, modulePath string) string {
	local, isLocal := localImportPath(imported, modulePath)
	if isLocal {
		if localImportAllowed(file, local) {
			return ""
		}
		return disallowedLocalImportReason(file, local)
	}

	switch {
	case hasPathPrefix(file, "actions"):
		switch {
		case imported == "net/http":
			return "actions must not use net/http directly"
		case imported == "github.com/spf13/cobra":
			return "actions must not import Cobra"
		}
	case hasPathPrefix(file, "internal/resources"):
		if imported == "net/http" {
			return "resource adapters must not use net/http directly"
		}
		if imported == "github.com/spf13/cobra" {
			return "resource adapters must not import Cobra"
		}
	case hasPathPrefix(file, "internal/tableau"):
		if imported == "github.com/spf13/cobra" {
			return "Tableau clients must not import Cobra"
		}
	case hasPathPrefix(file, "internal/cli"):
		if imported == "net/http" {
			return "CLI plumbing must not use net/http"
		}
	case isFoundationPackage(file):
		if imported == "github.com/spf13/cobra" {
			return "foundation packages must not import Cobra"
		}
	}
	return ""
}

type packageLayer string

const (
	layerUnknown       packageLayer = "unknown"
	layerAction        packageLayer = "action"
	layerApp           packageLayer = "app"
	layerCLI           packageLayer = "cli"
	layerResource      packageLayer = "resource"
	layerTableau       packageLayer = "tableau"
	layerFoundation    packageLayer = "foundation"
	layerTADXCommand   packageLayer = "tadx-command"
	layerDocsGenerator packageLayer = "docs-generator"
)

func localImportPath(imported, modulePath string) (string, bool) {
	if imported == modulePath {
		return "", true
	}
	prefix := modulePath + "/"
	if !strings.HasPrefix(imported, prefix) {
		return "", false
	}
	return strings.TrimPrefix(imported, prefix), true
}

func localImportAllowed(file, imported string) bool {
	switch layerForFile(file) {
	case layerAction:
		return matchesExact(imported,
			"internal/capability",
			"internal/config",
			"internal/errs",
			"internal/identity",
			"internal/output",
			"internal/pathspec",
		)
	case layerApp:
		return matchesPrefix(imported, "actions", "internal/cli", "internal/resources", "internal/tableau") ||
			matchesExact(imported,
				"internal/artifact",
				"internal/auth",
				"internal/capability",
				"internal/catalog",
				"internal/config",
				"internal/errs",
				"internal/identity",
				"internal/output",
				"internal/workspace",
			)
	case layerCLI:
		return matchesPrefix(imported, "actions", "internal/cli") || matchesExact(imported, "internal/errs", "internal/pathspec")
	case layerResource:
		return matchesExact(imported, "internal/identity") || matchesPrefix(imported, "internal/tableau")
	case layerTableau:
		return matchesExact(imported, "internal/auth", "internal/tableau", "internal/tableau/catalog/tabxml")
	case layerFoundation:
		if hasPathPrefix(file, "internal/output") {
			return matchesExact(imported, "internal/errs", "internal/toon")
		}
		if hasPathPrefix(file, "internal/workspace") {
			return matchesExact(imported, "internal/config")
		}
		// The artifact manager owns the workspace mutation critical section and
		// serializes it against other tadx processes via the leaf lock package. It
		// also asserts, at the destructive mutation boundary, that resolved artifact
		// paths cannot escape the workspace root, using the OS-independent pathspec
		// predicates as defense in depth over the upstream Resolve invariant.
		if hasPathPrefix(file, "internal/artifact") {
			return matchesExact(imported, "internal/lock", "internal/pathspec")
		}
		// The config package owns the user-configuration read-modify-write
		// critical section and serializes it against other tadx processes via
		// the leaf lock package so concurrent env/workspace updates cannot lose
		// writes.
		if hasPathPrefix(file, "internal/config") {
			return matchesExact(imported, "internal/lock")
		}
		return false
	case layerTADXCommand:
		return matchesExact(imported, "internal/app")
	case layerDocsGenerator:
		return matchesExact(imported, "internal/capability")
	default:
		return false
	}
}

func matchesExact(path string, allowed ...string) bool {
	for _, candidate := range allowed {
		if path == candidate {
			return true
		}
	}
	return false
}

func matchesPrefix(path string, allowed ...string) bool {
	for _, candidate := range allowed {
		if hasPathPrefix(path, candidate) {
			return true
		}
	}
	return false
}

func layerForFile(file string) packageLayer {
	switch {
	case hasPathPrefix(file, "actions"):
		return layerAction
	case hasPathPrefix(file, "internal/app"):
		return layerApp
	case hasPathPrefix(file, "internal/cli"):
		return layerCLI
	case hasPathPrefix(file, "internal/resources"):
		return layerResource
	case hasPathPrefix(file, "internal/tableau"):
		return layerTableau
	case isFoundationPackage(file):
		return layerFoundation
	case hasPathPrefix(file, "cmd/tadx"):
		return layerTADXCommand
	case hasPathPrefix(file, "cmd/gencapdocs"):
		return layerDocsGenerator
	default:
		return layerUnknown
	}
}

func disallowedLocalImportReason(file, imported string) string {
	switch layerForFile(file) {
	case layerAction:
		switch {
		case hasPathPrefix(imported, "actions"):
			return "actions must not import another action package"
		case hasPathPrefix(imported, "internal/app"):
			return "actions must not import the composition root"
		case hasPathPrefix(imported, "internal/auth"):
			return "actions must not import authentication logic"
		case hasPathPrefix(imported, "internal/cli"):
			return "actions must not import CLI packages"
		case hasPathPrefix(imported, "internal/resources"):
			return "actions must not import resource adapters"
		case hasPathPrefix(imported, "internal/tableau"):
			return "actions must not import Tableau clients"
		default:
			return "actions must not import unapproved local packages"
		}
	case layerApp:
		return "the composition root must not import unapproved local packages"
	case layerCLI:
		switch {
		case hasPathPrefix(imported, "internal/app"):
			return "CLI plumbing must not import the composition root"
		case hasPathPrefix(imported, "internal/auth"):
			return "CLI plumbing must not import authentication logic"
		case hasPathPrefix(imported, "internal/identity"):
			return "CLI plumbing must not import identity resolution"
		case hasPathPrefix(imported, "internal/resources"):
			return "CLI plumbing must not import resource adapters"
		case hasPathPrefix(imported, "internal/tableau"):
			return "CLI plumbing must not import Tableau clients"
		default:
			return "CLI plumbing must not import unapproved local packages"
		}
	case layerResource:
		switch {
		case hasPathPrefix(imported, "actions"):
			return "resource adapters must not import action packages"
		case hasPathPrefix(imported, "internal/app"):
			return "resource adapters must not import the composition root"
		case hasPathPrefix(imported, "internal/auth"):
			return "resource adapters must not import authentication logic"
		case hasPathPrefix(imported, "internal/cli"):
			return "resource adapters must not import CLI packages"
		default:
			return "resource adapters must not import unapproved local packages"
		}
	case layerTableau:
		switch {
		case hasPathPrefix(imported, "actions"):
			return "Tableau clients must not import action packages"
		case hasPathPrefix(imported, "internal/app"):
			return "Tableau clients must not import the composition root"
		case hasPathPrefix(imported, "internal/cli"):
			return "Tableau clients must not import CLI packages"
		case hasPathPrefix(imported, "internal/resources"):
			return "Tableau clients must not import resource adapters"
		default:
			return "Tableau clients must not import unapproved local packages"
		}
	case layerFoundation:
		if hasPathPrefix(imported, "actions") ||
			hasPathPrefix(imported, "internal/app") ||
			hasPathPrefix(imported, "internal/cli") ||
			hasPathPrefix(imported, "internal/resources") ||
			hasPathPrefix(imported, "internal/tableau") {
			return "foundation packages must not import higher layers"
		}
		return "foundation packages must not import unapproved local packages"
	case layerTADXCommand:
		return "cmd/tadx must not import unapproved local packages"
	case layerDocsGenerator:
		return "cmd/gencapdocs must not import unapproved local packages"
	default:
		return "unrecognized packages must not import local packages"
	}
}

func isFoundationPackage(file string) bool {
	return hasPathPrefix(file, "internal/artifact") ||
		hasPathPrefix(file, "internal/architecture") ||
		hasPathPrefix(file, "internal/auth") ||
		hasPathPrefix(file, "internal/config") ||
		hasPathPrefix(file, "internal/catalog") ||
		hasPathPrefix(file, "internal/identity") ||
		hasPathPrefix(file, "internal/capability") ||
		hasPathPrefix(file, "internal/errs") ||
		hasPathPrefix(file, "internal/lock") ||
		hasPathPrefix(file, "internal/output") ||
		hasPathPrefix(file, "internal/pathspec") ||
		hasPathPrefix(file, "internal/workspace") ||
		hasPathPrefix(file, "internal/toon")
}

func hasPathPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func readModulePath(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open go.mod: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	return "", fmt.Errorf("go.mod has no module directive")
}
