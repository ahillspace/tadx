//go:build fileinventory

package architecture_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// This inventory is deliberately opt-in: it is a repository-maintenance audit,
// not a product test. Run it with:
//
//	go test -tags=fileinventory ./internal/architecture -run TestGoFilesHaveResponsibilityClassification -v
//
// The supported release targets are the platforms exercised by the release
// matrix. The classification asks what responsibility a file serves and whether
// TADX needs to own that responsibility, rather than treating reachability as a
// complete explanation.
type fileNecessityCategory string

const (
	runtimeEssential    fileNecessityCategory = "runtime-essential"
	buildEssential      fileNecessityCategory = "build-essential"
	helpfulNonEssential fileNecessityCategory = "helpful-non-essential"
	nonEssential        fileNecessityCategory = "non-essential"
	uncertain           fileNecessityCategory = "uncertain"
	moduleImportPath                          = "github.com/ahillspace/tadx"
)

type fileNecessityEntry struct {
	category       fileNecessityCategory
	role           string
	responsibility string
	owner          string
	consumers      []string
	reason         string
	confidence     string
}

// These are the only non-test Go files currently outside the supported runtime
// dependency closure. Keeping this list explicit makes a new orphan fail the
// audit instead of silently becoming another unexplained file.
var nonRuntimeProductionFiles = map[string]fileNecessityEntry{
	"cmd/gencapdocs/main.go": {
		category:       buildEssential,
		role:           "repository generator",
		responsibility: "regenerates the committed capability reference artifacts from the registry",
		owner:          "TADX repository maintainers",
		consumers:      []string{"go:generate ./internal/capability"},
		reason:         "it is required for the maintained documentation-generation path but is not linked into the cmd/tadx binary.",
		confidence:     "high",
	},
	"internal/architecture/architecture.go": {
		category:       helpfulNonEssential,
		role:           "architecture checker",
		responsibility: "checks import boundaries while contributors change TADX",
		owner:          "TADX repository maintainers",
		consumers:      []string{"go test ./internal/architecture"},
		reason:         "it protects maintainability but is not needed by user behavior or the cmd/tadx build.",
		confidence:     "high",
	},
	"internal/guidancenotice/session_other.go": {
		category:       uncertain,
		role:           "platform compatibility fallback",
		responsibility: "provides parent-session lookup for GOOS values outside windows, linux, and darwin",
		owner:          "TADX platform-support decision",
		consumers:      []string{},
		reason:         "the build tag proves it is excluded from current release targets, but does not prove unsupported targets are irrelevant; review support policy before deletion.",
		confidence:     "low",
	},
}

type fileClassification struct {
	path           string
	packagePath    string
	category       fileNecessityCategory
	role           string
	lines          int
	consumers      []string
	responsibility string
	reason         string
	confidence     string
}

func TestGoFilesHaveResponsibilityClassification(t *testing.T) {
	root := repositoryRoot(t)
	files := repositoryGoFiles(t, root)
	runtimeFiles := runtimeFilesForSupportedTargets(t, root)
	packages := goListModulePackages(t, root)
	packageConsumers := packageConsumerIndex(packages)
	classifications := make([]fileClassification, 0, len(files))
	counts := make(map[fileNecessityCategory]int)
	unreviewedFiles := make([]string, 0)

	for path := range files {
		classification := classifyFile(path, root, runtimeFiles, packageConsumers)
		classifications = append(classifications, classification)
		counts[classification.category]++
		if classification.category == uncertain {
			if _, reviewed := nonRuntimeProductionFiles[path]; !reviewed && !strings.HasSuffix(path, "_test.go") {
				unreviewedFiles = append(unreviewedFiles, path)
			}
		}
	}
	for path, entry := range nonRuntimeProductionFiles {
		if _, ok := files[path]; !ok {
			t.Errorf("necessity entry %s (%s) does not name a repository Go file", path, entry.category)
		}
	}
	sort.Slice(classifications, func(i, j int) bool {
		return classifications[i].path < classifications[j].path
	})
	sort.Strings(unreviewedFiles)

	t.Logf("classified %d Go files (%d production, %d test)", len(classifications), productionFileCount(files), testFileCount(files))
	for _, category := range []fileNecessityCategory{
		runtimeEssential, buildEssential, helpfulNonEssential, nonEssential, uncertain,
	} {
		t.Logf("%s: %d files", category, counts[category])
	}
	for _, classification := range classifications {
		t.Logf("%s | %s | role=%s | lines=%d | package=%s | consumers=%s | confidence=%s | responsibility=%s | reason=%s",
			classification.category, classification.path, classification.role, classification.lines,
			classification.packagePath, strings.Join(classification.consumers, ","),
			classification.confidence, classification.responsibility, classification.reason)
	}
	if len(unreviewedFiles) > 0 {
		t.Fatalf("production Go files need an explicit responsibility and evidence review:\n%s", strings.Join(unreviewedFiles, "\n"))
	}
}

func classifyFile(path, root string, runtimeFiles map[string][]string, packageConsumers map[string][]string) fileClassification {
	classification := fileClassification{
		path:        path,
		packagePath: packagePathForFile(path),
		lines:       physicalLineCount(filepath.Join(root, filepath.FromSlash(path))),
		confidence:  "medium",
	}
	if entry, ok := nonRuntimeProductionFiles[path]; ok {
		classification.category = entry.category
		classification.role = entry.role
		classification.consumers = append([]string(nil), entry.consumers...)
		classification.responsibility = entry.responsibility
		classification.reason = entry.reason
		classification.confidence = entry.confidence
		return classification
	}
	if strings.HasSuffix(path, "_test.go") {
		classification.category = helpfulNonEssential
		classification.role = "test or fixture"
		classification.consumers = []string{"go test " + classification.packagePath}
		classification.responsibility = "verifies or supplies fixtures for the owning package"
		classification.reason = "not shipped in the user binary, but it protects TADX behavior during creation and maintenance"
		classification.confidence = "high"
		return classification
	}
	if targets, ok := runtimeFiles[path]; ok {
		classification.category = runtimeEssential
		classification.role = "user-facing runtime"
		classification.consumers = []string{"cmd/tadx [" + strings.Join(targets, ",") + "]"}
		classification.responsibility = "supports supported TADX user behavior or its platform runtime"
		classification.reason = "compiled into cmd/tadx for at least one supported release target"
		classification.confidence = "high"
		return classification
	}
	classification.category = uncertain
	classification.role = "unclassified production source"
	classification.consumers = packageConsumers[classification.packagePath]
	if len(classification.consumers) == 0 {
		classification.consumers = []string{"none found by go list"}
	}
	classification.responsibility = "unknown"
	classification.reason = "outside supported cmd/tadx builds without a reviewed ownership decision"
	classification.confidence = "low"
	return classification
}

type goListPackage struct {
	ImportPath      string
	Dir             string
	GoFiles         []string
	CgoFiles        []string
	CompiledGoFiles []string
	TestGoFiles     []string
	XTestGoFiles    []string
	Imports         []string
	TestImports     []string
	XTestImports    []string
	Error           *goListError
}

type goListError struct {
	Err string
}

func repositoryGoFiles(t *testing.T, root string) map[string]struct{} {
	t.Helper()
	files := make(map[string]struct{})
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && ignoredInventoryDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("scan repository Go files: %v", err)
	}
	return files
}

func ignoredInventoryDirectory(name string) bool {
	switch name {
	case ".git", ".tadx-explore", "archived", "jev-evals", "vendor":
		return true
	default:
		return false
	}
}

func runtimeFilesForSupportedTargets(t *testing.T, root string) map[string][]string {
	t.Helper()
	targets := make(map[string][]string)
	for _, goos := range []string{"windows", "linux", "darwin"} {
		for _, pkg := range goListTADXDependencies(t, root, goos) {
			if pkg.Error != nil || !isTADXImport(pkg.ImportPath) {
				continue
			}
			names := pkg.CompiledGoFiles
			if len(names) == 0 {
				names = append(append([]string{}, pkg.GoFiles...), pkg.CgoFiles...)
			}
			for _, name := range names {
				path := name
				if !filepath.IsAbs(path) {
					path = filepath.Join(pkg.Dir, path)
				}
				relative, err := filepath.Rel(root, path)
				if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					continue
				}
				relative = filepath.ToSlash(relative)
				if !strings.HasSuffix(relative, ".go") || strings.HasSuffix(relative, "_test.go") {
					continue
				}
				targets[relative] = append(targets[relative], goos)
			}
		}
	}
	for path := range targets {
		sort.Strings(targets[path])
	}
	return targets
}

func goListTADXDependencies(t *testing.T, root, goos string) []goListPackage {
	t.Helper()
	return runGoList(t, root, map[string]string{
		"GOOS":        goos,
		"GOARCH":      "amd64",
		"CGO_ENABLED": "0",
	}, "-deps", "./cmd/tadx")
}

func goListModulePackages(t *testing.T, root string) []goListPackage {
	t.Helper()
	return runGoList(t, root, map[string]string{}, "./...")
}

func runGoList(t *testing.T, root string, overrides map[string]string, args ...string) []goListPackage {
	t.Helper()
	command := exec.Command("go", append([]string{"list", "-json"}, args...)...)
	command.Dir = root
	command.Env = environment(overrides)
	output, err := command.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list %s: %v\n%s", strings.Join(args, " "), err, exit.Stderr)
		}
		t.Fatalf("go list %s: %v", strings.Join(args, " "), err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	var packages []goListPackage
	for {
		var pkg goListPackage
		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode go list %s: %v", strings.Join(args, " "), err)
		}
		packages = append(packages, pkg)
	}
	return packages
}

func packageConsumerIndex(packages []goListPackage) map[string][]string {
	consumers := make(map[string]map[string]struct{})
	for _, pkg := range packages {
		for _, imported := range append(append(append([]string{}, pkg.Imports...), pkg.TestImports...), pkg.XTestImports...) {
			if !isTADXImport(imported) || imported == pkg.ImportPath {
				continue
			}
			if consumers[imported] == nil {
				consumers[imported] = make(map[string]struct{})
			}
			consumers[imported][pkg.ImportPath] = struct{}{}
		}
	}
	result := make(map[string][]string, len(consumers))
	for imported, values := range consumers {
		for consumer := range values {
			result[imported] = append(result[imported], consumer)
		}
		sort.Strings(result[imported])
	}
	return result
}

func physicalLineCount(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	if len(data) == 0 {
		return 0
	}
	lines := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		lines++
	}
	return lines
}

func packagePathForFile(path string) string {
	directory := filepath.ToSlash(filepath.Dir(path))
	if directory == "." {
		return moduleImportPath
	}
	return moduleImportPath + "/" + directory
}

func productionFileCount(files map[string]struct{}) int {
	count := 0
	for path := range files {
		if !strings.HasSuffix(path, "_test.go") {
			count++
		}
	}
	return count
}

func testFileCount(files map[string]struct{}) int {
	return len(files) - productionFileCount(files)
}

func environment(overrides map[string]string) []string {
	values := make(map[string]string)
	for _, value := range os.Environ() {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) == 2 {
			values[parts[0]] = parts[1]
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, fmt.Sprintf("%s=%s", key, values[key]))
	}
	return result
}

func isTADXImport(path string) bool {
	return path == moduleImportPath || strings.HasPrefix(path, moduleImportPath+"/")
}
