package architecture_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestRepositoryRespectsImportBoundaries(t *testing.T) {
	root := repositoryRoot(t)
	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(violations) > 0 {
		var lines []string
		for _, violation := range violations {
			lines = append(lines, violation.String())
		}
		t.Fatalf("architecture violations:\n%s", strings.Join(lines, "\n"))
	}
}

func TestFirstPartyFilesDoNotContainDeveloperHomePaths(t *testing.T) {
	root := repositoryRoot(t)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)[a-z]:[\\/]users[\\/][^\\/\s]+`),
		regexp.MustCompile(`/` + `Users/[^/\s]+`),
		regexp.MustCompile(`/` + `home/[^/\s]+`),
	}
	excludedDirectories := map[string]bool{
		".git": true, ".agents": true, ".tadx-explore": true, "agent-review": true, "archived": true,
		"Tableau API Documentation": true,
	}
	textExtensions := map[string]bool{
		".go": true, ".md": true, ".yaml": true, ".yml": true, ".json": true, ".toon": true,
	}
	var violations []string
	source, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open source root: %v", err)
	}
	defer source.Close()
	files, err := architecture.FirstPartyFiles(root)
	if err != nil {
		t.Fatalf("select first-party files: %v", err)
	}
	for _, relative := range files {
		excluded := strings.HasPrefix(relative, "internal/toon/testdata/upstream-v4.1.1/")
		for part := range strings.SplitSeq(relative, "/") {
			if excludedDirectories[part] {
				excluded = true
				break
			}
		}
		if excluded || !textExtensions[strings.ToLower(filepath.Ext(relative))] {
			continue
		}
		content, err := source.ReadFile(filepath.FromSlash(relative))
		if err != nil {
			t.Fatalf("read first-party file %s: %v", relative, err)
		}
		for _, pattern := range patterns {
			if pattern.Match(content) {
				violations = append(violations, relative)
				break
			}
		}
	}
	if len(violations) > 0 {
		t.Fatalf("developer home paths found in first-party files:\n%s", strings.Join(violations, "\n"))
	}
}

func TestCheckRejectsActionImports(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "actions/workbook/pull/action.go", `package pull
import (
	"net/http"
	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
	"example.test/tadx/actions/workbook/get"
	"example.test/tadx/internal/app"
	"example.test/tadx/internal/auth"
	"example.test/tadx/internal/cli"
	"example.test/tadx/internal/resources/workbook"
	"example.test/tadx/internal/tableau"
)
var _ = http.MethodGet
var _ = resty.New
var _ = cobra.NoArgs
var _ = get.Output{}
var _ = app.Run
var _ auth.Provider
var _ = cli.NewRoot
var _ = workbook.Adapter{}
var _ tableau.Client
`)

	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertReasons(t, violations, []string{
		"actions must not import another action package",
		"actions must not import the composition root",
		"actions must not import authentication logic",
		"actions must not import CLI packages",
		"actions must not import resource adapters",
		"actions must not import Tableau clients",
		"actions must not import Cobra",
		"actions must not use net/http directly",
	})
}

func TestCheckUsesFirstPartySourceOwnership(t *testing.T) {
	root := moduleFixture(t)
	git := func(args ...string) {
		t.Helper()
		command := exec.CommandContext(t.Context(), "git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	git("init", "--quiet")
	writeGo(t, root, ".gitignore", "/scratch/\n*.ignored.go\n")
	writeGo(t, root, "actions/example/action.go", "package example\nimport _ \"fmt\"\n")
	writeGo(t, root, "main.go", "package main\nimport _ \"fmt\"\n")
	writeGo(t, root, "deleted/source.go", "package deleted\n")
	git("add", "go.mod", ".gitignore", "main.go", "actions/example/action.go", "deleted/source.go")
	if err := os.Remove(filepath.Join(root, "deleted", "source.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "deleted")); err != nil {
		t.Fatal(err)
	}
	writeGo(t, root, "extra.ignored.go", "package main\nimport _ \"example.test/tadx/internal/config\"\n")
	writeGo(t, root, "actions/example/extra.ignored.go", "package example\nimport _ \"net/http\"\n")
	writeGo(t, root, "actions/new/action.ignored.go", "package new\nimport _ \"net/http\"\n")
	writeGo(t, root, "newpackage/source.go", "package newpackage\nimport _ \"example.test/tadx/internal/config\"\n")
	writeGo(t, root, "scratch/copy.go", "not Go source")
	writeGo(t, root, "extensions/editor/node_modules/dependency/example.go", "not Go source")
	writeGo(t, root, "tools/separate/go.mod", "module example.test/separate\n\ngo 1.26\n")
	writeGo(t, root, "tools/separate/main.go", "package main\nimport _ \"example.test/tadx/internal/config\"\n")

	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertViolationStrings(t, violations, []string{
		"actions/example/extra.ignored.go imports net/http: actions must not use net/http directly",
		"actions/new/action.ignored.go imports net/http: actions must not use net/http directly",
		"extra.ignored.go imports example.test/tadx/internal/config: unrecognized packages must not import local packages",
		"newpackage/source.go imports example.test/tadx/internal/config: unrecognized packages must not import local packages",
	})
}

func TestCheckSourceExportPreservesUnknownPackages(t *testing.T) {
	root := moduleFixture(t)
	for _, directory := range []string{".scratch", "_build", "vendor", "node_modules", "archived"} {
		writeGo(t, root, directory+"/copy.go", "not Go source")
	}
	writeGo(t, root, "newpackage/source.go", "package newpackage\nimport _ \"example.test/tadx/internal/config\"\n")
	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertViolationStrings(t, violations, []string{
		"newpackage/source.go imports example.test/tadx/internal/config: unrecognized packages must not import local packages",
	})
}

func TestCheckFollowsExplicitImportsIntoGoExcludedDirectories(t *testing.T) {
	for _, directory := range []string{"_helper", ".hidden"} {
		t.Run(directory, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, "internal/app/app.go", "package app\nimport _ \"example.test/tadx/internal/resources/"+directory+"\"\n")
			writeGo(t, root, "internal/resources/"+directory+"/source.go", "package helper\nimport _ \"net/http\"\n")
			command := exec.CommandContext(t.Context(), "go", "list", "-deps", "./internal/app")
			command.Dir = root
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("Go rejected explicit fixture import: %v\n%s", err, output)
			}
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, []string{
				"internal/resources/" + directory + "/source.go imports net/http: resource adapters must not use net/http directly",
			})
			files, err := architecture.FirstPartyFiles(root)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(files, "internal/resources/"+directory+"/source.go") {
				t.Fatal("first-party content checks omitted an explicitly imported source file")
			}
		})
	}
}

func TestCheckReportsBrokenGitMetadata(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, ".git", "gitdir: missing-git-directory\n")
	if _, err := architecture.Check(root); err == nil {
		t.Fatal("Check accepted unreadable Git ownership metadata")
	}
}

func TestCheckReportsMalformedNestedModule(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "internal/new/go.mod", "not a module declaration\n")
	writeGo(t, root, "internal/new/source.go", "package new\nimport _ \"example.test/tadx/internal/config\"\n")
	if _, err := architecture.Check(root); err == nil {
		t.Fatal("Check treated malformed module evidence as a separate ownership boundary")
	}
}

func TestCheckRejectsNonregularGoSource(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "target.txt", "package fixture\n")
	if err := os.Symlink(filepath.Join(root, "target.txt"), filepath.Join(root, "source.go")); err != nil {
		t.Skipf("source symlink requires platform support and privileges: %v", err)
	}
	if _, err := architecture.Check(root); err == nil {
		t.Fatal("Check silently accepted nonregular Go source")
	}
}

func TestCheckAllowsThirdPartyImportsInActionsAndAdapters(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "actions/workbook/pull/action.go", `package pull
import "github.com/go-resty/resty/v2"
var _ = resty.New
`)
	writeGo(t, root, "internal/resources/workbook/adapter.go", `package workbook
import "github.com/go-resty/resty/v2"
var _ = resty.New
`)

	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertViolationStrings(t, violations, nil)
}

func TestCheckRejectsUnapprovedLocalImportsFromEveryLayer(t *testing.T) {
	root := moduleFixture(t)
	for _, file := range []string{
		"actions/workbook/pull/action.go",
		"cmd/gencapdocs/main.go",
		"cmd/tadx/main.go",
		"internal/app/app.go",
		"internal/cli/workbook/command.go",
		"internal/config/config.go",
		"internal/mystery/source.go",
		"internal/resources/workbook/adapter.go",
		"internal/tableau/workbook/client.go",
	} {
		writeGo(t, root, file, `package fixture
import "example.test/tadx/internal/unapproved"
var _ = unapproved.Value
`)
	}

	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertViolationStrings(t, violations, []string{
		"actions/workbook/pull/action.go imports example.test/tadx/internal/unapproved: actions must not import unapproved local packages",
		"cmd/gencapdocs/main.go imports example.test/tadx/internal/unapproved: cmd/gencapdocs must not import unapproved local packages",
		"cmd/tadx/main.go imports example.test/tadx/internal/unapproved: cmd/tadx must not import unapproved local packages",
		"internal/app/app.go imports example.test/tadx/internal/unapproved: the composition root must not import unapproved local packages",
		"internal/cli/workbook/command.go imports example.test/tadx/internal/unapproved: CLI plumbing must not import unapproved local packages",
		"internal/config/config.go imports example.test/tadx/internal/unapproved: foundation packages must not import unapproved local packages",
		"internal/mystery/source.go imports example.test/tadx/internal/unapproved: unrecognized packages must not import local packages",
		"internal/resources/workbook/adapter.go imports example.test/tadx/internal/unapproved: resource adapters must not import unapproved local packages",
		"internal/tableau/workbook/client.go imports example.test/tadx/internal/unapproved: Tableau clients must not import unapproved local packages",
	})
}

func TestCheckAllowsDocumentedLocalImports(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "actions/workbook/get/action.go", `package get
import (
	_ "example.test/tadx/internal/capability"
	_ "example.test/tadx/internal/config"
	_ "example.test/tadx/internal/errs"
	_ "example.test/tadx/internal/identity"
	_ "example.test/tadx/internal/output"
)
`)
	writeGo(t, root, "internal/app/app.go", `package app
import (
	_ "example.test/tadx/actions/workbook/get"
	_ "example.test/tadx/internal/auth"
	_ "example.test/tadx/internal/capability"
	_ "example.test/tadx/internal/cli"
	_ "example.test/tadx/internal/config"
	_ "example.test/tadx/internal/errs"
	_ "example.test/tadx/internal/output"
	_ "example.test/tadx/internal/resources/workbook"
	_ "example.test/tadx/internal/tableau/workbook"
	_ "example.test/tadx/internal/version"
)
`)
	writeGo(t, root, "internal/cli/root.go", `package cli
import (
	_ "example.test/tadx/actions/workbook/get"
	_ "example.test/tadx/internal/cli/workbook"
	_ "example.test/tadx/internal/errs"
)
`)
	writeGo(t, root, "internal/resources/workbook/adapter.go", `package workbook
import (
	_ "example.test/tadx/internal/identity"
	_ "example.test/tadx/internal/tableau/workbook"
)
`)
	writeGo(t, root, "internal/tableau/workbook/client.go", `package workbook
import (
	_ "example.test/tadx/internal/auth"
	_ "example.test/tadx/internal/tableau"
)
`)
	writeGo(t, root, "internal/tableau/cache/client.go", `package cache
import _ "example.test/tadx/internal/tableau/cache/tabxml"
`)
	writeGo(t, root, "internal/output/output.go", `package output
import (
	_ "example.test/tadx/internal/errs"
	_ "example.test/tadx/internal/toon"
)
`)
	writeGo(t, root, "internal/workspace/manager.go", `package workspace
import _ "example.test/tadx/internal/config"
`)
	writeGo(t, root, "internal/operationrun/store.go", `package operationrun
import _ "example.test/tadx/internal/lock"
`)
	writeGo(t, root, "cmd/tadx/main.go", `package main
import _ "example.test/tadx/internal/app"
`)
	writeGo(t, root, "cmd/gencapdocs/main.go", `package main
import _ "example.test/tadx/internal/capability"
`)

	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertViolationStrings(t, violations, nil)
}

func TestCheckAllowsOperationRunThroughCompositionRoot(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "internal/app/worker.go", `package app
import _ "example.test/tadx/internal/operationrun"
`)
	writeGo(t, root, "internal/operationrun/store.go", `package operationrun
import _ "example.test/tadx/internal/lock"
`)

	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertViolationStrings(t, violations, nil)
}

func TestCheckAllowsLocalStateWritersToUseReplaceLeaf(t *testing.T) {
	root := moduleFixture(t)
	for _, file := range []string{
		"internal/agent/install.go",
		"internal/artifact/workbook.go",
		"internal/config/config.go",
		"internal/jobmonitor/store.go",
		"internal/lastcommand/store.go",
		"internal/operationrun/store.go",
		"internal/workspace/manager.go",
	} {
		name := strings.Split(file, "/")[1]
		writeGo(t, root, file, "package "+name+"\nimport _ \"example.test/tadx/internal/fsreplace\"\n")
	}

	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertViolationStrings(t, violations, nil)
}

func TestCheckKeepsReplaceLeafFreeOfLocalImports(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "internal/fsreplace/fsreplace.go", `package fsreplace
import _ "example.test/tadx/internal/lock"
`)

	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertViolationStrings(t, violations, []string{
		"internal/fsreplace/fsreplace.go imports example.test/tadx/internal/lock: foundation packages must not import unapproved local packages",
	})
}

func TestCheckRejectsOperationRunBoundaryBypass(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "internal/app/worker.go", `package app
import _ "example.test/tadx/internal/lock"
`)
	writeGo(t, root, "internal/operationrun/store.go", `package operationrun
import _ "example.test/tadx/internal/config"
`)

	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertViolationStrings(t, violations, []string{
		"internal/app/worker.go imports example.test/tadx/internal/lock: the composition root must not import unapproved local packages",
		"internal/operationrun/store.go imports example.test/tadx/internal/config: operation-run infrastructure must import only the lock and replace boundaries",
	})
}

func TestCheckRejectsImportsFromEveryFoundationPackage(t *testing.T) {
	foundations := []struct {
		path string
		name string
	}{
		{path: "internal/architecture", name: "architecture"},
		{path: "internal/auth", name: "auth"},
		{path: "internal/capability", name: "capability"},
		{path: "internal/config", name: "config"},
		{path: "internal/errs", name: "errs"},
		{path: "internal/fsreplace", name: "fsreplace"},
		{path: "internal/identity", name: "identity"},
		{path: "internal/output", name: "output"},
		{path: "internal/workspace", name: "workspace"},
		{path: "internal/toon", name: "toon"},
	}

	for _, foundation := range foundations {
		t.Run(foundation.name, func(t *testing.T) {
			root := moduleFixture(t)
			contents := fmt.Sprintf(`package %s
import (
	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
	"example.test/tadx/internal/app"
)
var _ = cobra.NoArgs
var _ = resty.New
var _ = app.Run
`, foundation.name)
			writeGo(t, root, foundation.path+"/foundation.go", contents)

			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertReasons(t, violations, []string{
				"foundation packages must not import higher layers",
				"foundation packages must not import Cobra",
			})
		})
	}
}

func TestCheckRejectsFoundationImports(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "internal/config/config.go", `package config
import (
	"github.com/spf13/cobra"
	"example.test/tadx/actions/workbook/get"
	"example.test/tadx/internal/app"
	"example.test/tadx/internal/cli"
	"example.test/tadx/internal/resources/workbook"
	"example.test/tadx/internal/tableau"
)
var _ = cobra.NoArgs
var _ = get.Output{}
var _ = app.Run
var _ = cli.NewRoot
var _ = workbook.Adapter{}
var _ tableau.Client
`)

	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertReasons(t, violations, []string{
		"foundation packages must not import higher layers",
		"foundation packages must not import higher layers",
		"foundation packages must not import higher layers",
		"foundation packages must not import higher layers",
		"foundation packages must not import higher layers",
		"foundation packages must not import Cobra",
	})
}

func TestCheckRejectsAdapterAndTransportImports(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "internal/resources/workbook/adapter.go", `package workbook
import (
	"net/http"
	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
	"example.test/tadx/actions/workbook/get"
	"example.test/tadx/internal/app"
	"example.test/tadx/internal/auth"
	"example.test/tadx/internal/cli"
)
var _ = http.MethodGet
var _ = resty.New
var _ = cobra.NoArgs
var _ = get.Output{}
var _ = app.Run
var _ auth.Provider
var _ = cli.NewRoot
`)
	writeGo(t, root, "internal/tableau/transport.go", `package tableau
import (
	"example.test/tadx/internal/app"
	"example.test/tadx/internal/resources/workbook"
)
var _ = app.Run
var _ = workbook.Adapter{}
`)

	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertReasons(t, violations, []string{
		"resource adapters must not import action packages",
		"resource adapters must not import the composition root",
		"resource adapters must not import authentication logic",
		"resource adapters must not import CLI packages",
		"resource adapters must not import Cobra",
		"resource adapters must not use net/http directly",
		"Tableau clients must not import the composition root",
		"Tableau clients must not import resource adapters",
	})
}

func TestCheckRejectsDomainLogicImportsFromCLI(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "internal/cli/root.go", `package cli
import (
	"net/http"
	"example.test/tadx/internal/app"
	"example.test/tadx/internal/auth"
	"example.test/tadx/internal/identity"
)
var _ = http.MethodGet
var _ = app.Run
var _ auth.Provider
var _ = identity.Selector{}
`)

	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertReasons(t, violations, []string{
		"CLI plumbing must not import the composition root",
		"CLI plumbing must not import authentication logic",
		"CLI plumbing must not import identity resolution",
		"CLI plumbing must not use net/http",
	})
}

func moduleFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/tadx\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeGo(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertReasons(t *testing.T, violations []architecture.Violation, want []string) {
	t.Helper()
	got := make([]string, len(violations))
	for i, violation := range violations {
		got[i] = violation.Reason
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("reasons:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func assertViolationStrings(t *testing.T, violations []architecture.Violation, want []string) {
	t.Helper()
	got := make([]string, len(violations))
	for i, violation := range violations {
		got[i] = violation.String()
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("violations:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
