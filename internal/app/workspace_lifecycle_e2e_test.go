package app_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/workspace"
)

func TestWorkspaceDeletionThroughCLIRespectsActiveLifecycleOwners(t *testing.T) {
	for _, owner := range []string{"artifact", "download"} {
		t.Run(owner, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "target")
			options := app.Options{ConfigPath: filepath.Join(base, "config.yaml")}
			run := func(args ...string) (int, string) {
				var output bytes.Buffer
				code := app.Run(t.Context(), args, &output, options)
				return code, output.String()
			}
			for _, args := range [][]string{{"workspace", "create", "default", "--path", filepath.Join(base, "default")}, {"workspace", "create", "target", "--path", root}} {
				if code, out := run(args...); code != 0 {
					t.Fatal(out)
				}
			}
			var release func()
			if owner == "artifact" {
				mutation, err := workspace.BeginMutation(t.Context(), root)
				if err != nil {
					t.Fatal(err)
				}
				release = func() { _ = mutation.Release() }
			} else {
				downloads := workspace.NewDownloads(root)
				if _, err := downloads.File(t.Context(), func(w io.Writer) error { _, err := io.WriteString(w, "download in progress"); return err }); err != nil {
					t.Fatal(err)
				}
				release = downloads.Close
			}
			defer release()
			for _, force := range []bool{false, true} {
				args := []string{"workspace", "delete", "target"}
				if force {
					args = append(args, "--force")
				}
				if code, out := run(args...); code == 0 || !strings.Contains(out, "workspace is in use") {
					t.Fatalf("active %s, force=%t: code=%d %s", owner, force, code, out)
				}
				if _, err := os.Stat(filepath.Join(root, "tadx.yaml")); err != nil {
					t.Fatalf("active workspace changed: %v", err)
				}
			}
			release()
			if code, out := run("workspace", "delete", "target"); code != 0 {
				t.Fatalf("delete after owner completed: %s", out)
			}
		})
	}
}

func TestWorkspaceDeletionAfterNestedWorkspaceRemovalThroughCLI(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "parent")
	options := app.Options{ConfigPath: filepath.Join(base, "config.yaml")}
	run := func(args ...string) (int, string) {
		var output bytes.Buffer
		code := app.Run(t.Context(), args, &output, options)
		return code, output.String()
	}
	for _, args := range [][]string{
		{"workspace", "create", "default", "--path", filepath.Join(base, "default")},
		{"workspace", "create", "parent", "--path", parent},
		{"workspace", "create", "child", "--path", filepath.Join(parent, "child")},
		{"workspace", "delete", "child"},
	} {
		if code, out := run(args...); code != 0 {
			t.Fatal(out)
		}
	}
	if code, out := run("workspace", "delete", "parent", "--preview"); code != 0 || !strings.Contains(out, "dirty: false") {
		t.Fatalf("empty child coordination state marked parent dirty: %s", out)
	}
	if code, out := run("workspace", "delete", "parent"); code != 0 {
		t.Fatalf("clean parent deletion after child removal: %s", out)
	}
}
