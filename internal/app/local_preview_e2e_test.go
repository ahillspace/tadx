package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/artifact"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func TestWorkspaceCreatePreviewDoesNotCreateOrRegister(t *testing.T) {
	root := t.TempDir()
	opts := overviewOptions(t, root)
	destination := filepath.Join(root, "proposed")
	code, out := runPreviewCommand(t, []string{"workspace", "create", "example", "--path", destination, "--preview"}, opts)
	if code != 0 || !strings.Contains(out, "preview") {
		t.Fatalf("code=%d output=%s", code, out)
	}
	for _, path := range []string{destination, opts.ConfigPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("preview wrote %s: %v", path, err)
		}
	}
}

func TestWorkspaceLocalPreviewsPreserveState(t *testing.T) {
	root := t.TempDir()
	opts := overviewOptions(t, root)
	manager := workspacecore.NewManager(opts.ConfigPath, nil)
	source := filepath.Join(root, "source")
	other := filepath.Join(root, "other")
	for name, path := range map[string]string{"source": source, "other": other} {
		if _, err := manager.Create(t.Context(), name, path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.SetDefault(t.Context(), "source"); err != nil {
		t.Fatal(err)
	}
	_, err := artifact.NewWorkbookManager(nil).Pull(t.Context(), artifact.WorkbookPull{Workspace: source, Filename: "Sales.twb", Content: []byte("<workbook/>"), Metadata: artifact.WorkbookMetadata{Name: "Sales", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "example", SourceSite: "example", SourceProjectName: "Analytics", SourceProjectID: "project-1"}})
	if err != nil {
		t.Fatal(err)
	}
	unregistered := filepath.Join(root, "unregistered")
	independent := workspacecore.NewManager(filepath.Join(t.TempDir(), "config.yaml"), nil)
	if _, err := independent.Create(t.Context(), "adopt", unregistered); err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(source, ".tadx", "logs")
	if err := os.MkdirAll(logs, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "example.log"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	configBefore, err := os.ReadFile(opts.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	before := map[string]map[string]string{}
	for _, path := range []string{source, other, unregistered} {
		before[path] = overviewSnapshot(t, path)
	}
	tests := [][]string{
		{"workspace", "clone", "source", "--name", "copy", "--path", filepath.Join(root, "copy")},
		{"workspace", "register", "--path", unregistered},
		{"workspace", "clean", "--workspace", "source", "--class", "logs"},
		{"workspace", "artifact", "move", "--source", "source", "--destination", "other", "--kind", "workbook", "--id", "wb-1"},
		{"workspace", "set-default", "other"},
		{"workspace", "unregister", "other"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args[:2], "-"), func(t *testing.T) {
			code, out := runPreviewCommand(t, append(args, "--preview", "--full"), opts)
			if code != 0 || !strings.Contains(out, "preview") {
				t.Fatalf("code=%d output=%s", code, out)
			}
			current, err := os.ReadFile(opts.ConfigPath)
			if err != nil || string(current) != string(configBefore) {
				t.Fatalf("configuration changed: %v", err)
			}
			for path, snapshot := range before {
				if !reflect.DeepEqual(snapshot, overviewSnapshot(t, path)) {
					t.Fatalf("preview changed %s", path)
				}
			}
			if _, err := os.Stat(filepath.Join(root, "copy")); !os.IsNotExist(err) {
				t.Fatalf("clone destination created: %v", err)
			}
		})
	}
	code, out := runPreviewCommand(t, []string{"workspace", "create", "source", "--path", filepath.Join(root, "collision"), "--preview"}, opts)
	if code == 0 || !strings.Contains(out, "already exists") {
		t.Fatalf("collision code=%d output=%s", code, out)
	}
	code, out = runPreviewCommand(t, []string{"workspace", "create", "actual", "--path", filepath.Join(root, "actual"), "--preview=false"}, opts)
	if code != 0 {
		t.Fatalf("execute code=%d output=%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(root, "actual", "tadx.yaml")); err != nil {
		t.Fatal(err)
	}
}

func runPreviewCommand(t *testing.T, args []string, opts Options) (int, string) {
	t.Helper()
	var out strings.Builder
	code := Run(t.Context(), args, &out, opts)
	return code, out.String()
}
