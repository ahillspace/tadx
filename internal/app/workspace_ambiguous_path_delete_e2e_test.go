package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func TestWorkspaceDeleteRejectsUnknownRegisteredPathThroughCLI(t *testing.T) {
	for _, test := range []struct {
		name       string
		pathFields func(string, string) string
	}{
		{"duplicate identical paths", func(child, _ string) string { return "    path: " + child + "\n    path: " + child + "\n" }},
		{"duplicate conflicting paths", func(child, outside string) string { return "    path: " + child + "\n    path: " + outside + "\n" }},
		{"malformed path", func(child, _ string) string { return "    path: [" + child + "]\n" }},
		{"absent path", func(_, _ string) string { return "" }},
		{"blank path", func(_, _ string) string { return "    path: \"\"\n" }},
		{"whitespace path", func(_, _ string) string { return "    path: \"   \"\n" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			path := filepath.Join(base, "config.yaml")
			options := Options{ConfigPath: path, PATStore: &fakePATStore{}}
			root := filepath.Join(base, "parent")
			resilienceRequire(t, options, "--config", path, "workspace", "create", "parent", "--path", root, "--json")
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			parent := cfg.Workspaces["parent"]
			child := filepath.Join(root, "child")
			if err := os.Mkdir(child, 0o700); err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{filepath.Join(root, "keep.txt"), filepath.Join(child, "keep.txt")} {
				if err := os.WriteFile(marker, []byte("preserve"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			manifestPath := filepath.Join(root, config.WorkspaceConfigName)
			manifest, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			// The parent has no default reference, so that independent guard cannot mask path protection.
			before := fmt.Appendf(nil, "version: 1\nworkspaces:\n  parent:\n    id: %s\n    path: %s\n  child:\n    id: ws_22222222222222222222222222222222\n%s", parent.ID, strconv.Quote(filepath.ToSlash(root)), test.pathFields(strconv.Quote(filepath.ToSlash(child)), strconv.Quote(filepath.ToSlash(filepath.Join(base, "outside")))))
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err = config.Load(path)
			if err != nil || cfg.DefaultWorkspace != "" || len(cfg.InvalidWorkspaces) != 1 || strings.TrimSpace(cfg.WorkspaceRegistrations()["child"].Path) != "" {
				t.Fatalf("fixture did not isolate an unknown child path: config=%+v err=%v", cfg, err)
			}
			parent = cfg.Workspaces["parent"]
			if _, err := workspacecore.NewManager(path, nil).Resolve(t.Context(), "parent", ""); err != nil {
				t.Fatalf("unknown child path blocked healthy parent resolution: %v", err)
			}
			resilienceRequire(t, options, "--config", path, "workspace", "list", "--json")
			resilienceRequire(t, options, "--config", path, "workspace", "delete", "parent", "--preview", "--json")
			code, out := resilienceRun(t, options, "--config", path, "workspace", "delete", "parent", "--force", "--json")
			if code == 0 || !strings.Contains(out, "no unambiguous path") || !strings.Contains(out, "child") {
				t.Errorf("destructive deletion did not reject the unknown child location: code=%d output=%s", code, out)
			}
			if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, before) {
				t.Error("rejected deletion changed configuration bytes", err)
			}
			if data, err := os.ReadFile(manifestPath); err != nil || !bytes.Equal(data, manifest) {
				t.Error("rejected deletion changed the parent manifest", err)
			}
			for _, marker := range []string{filepath.Join(root, "keep.txt"), filepath.Join(child, "keep.txt")} {
				if data, err := os.ReadFile(marker); err != nil || string(data) != "preserve" {
					t.Errorf("rejected deletion lost preserved files: path=%s err=%v", marker, err)
				}
			}
			if _, err := os.Lstat(filepath.Join(base, ".tadx-workspace-delete-"+parent.ID)); !os.IsNotExist(err) {
				t.Errorf("rejected deletion left a staged parent directory: %v", err)
			}
			cfg, err = config.Load(path)
			if err != nil || cfg.Workspaces["parent"] != parent || len(cfg.InvalidWorkspaces) != 1 {
				t.Errorf("rejected deletion changed registrations: config=%+v err=%v", cfg, err)
			}
			if t.Failed() {
				return
			}
			// Removing the invalid registration remains possible and preserves its files.
			resilienceRequire(t, options, "--config", path, "workspace", "unregister", "child", "--json")
			if data, err := os.ReadFile(filepath.Join(child, "keep.txt")); err != nil || string(data) != "preserve" {
				t.Fatal("registration repair changed child files", err)
			}
			resilienceRequire(t, options, "--config", path, "workspace", "delete", "parent", "--force", "--json")
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatal("repaired configuration did not permit ordinary deletion", err)
			}
			cfg, err = config.Load(path)
			if err != nil || len(cfg.WorkspaceNames()) != 0 {
				t.Fatal("ordinary deletion did not remove the parent registration", err)
			}
		})
	}
}
