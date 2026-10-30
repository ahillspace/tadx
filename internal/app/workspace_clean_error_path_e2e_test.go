package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestWorkspaceCleanLockFailureDoesNotRenderAbsoluteWorkspacePaths(t *testing.T) {
	directory := t.TempDir()
	root := filepath.Join(directory, "private workspace")
	options := app.Options{ConfigPath: filepath.Join(directory, "config.yaml")}
	var created bytes.Buffer
	if code := app.Run(t.Context(), []string{"workspace", "create", "example", "--path", root}, &created, options); code != 0 {
		t.Fatalf("create: %s", created.String())
	}
	blocked := filepath.Join(root, ".tadx", "tmp.lock")
	if err := os.MkdirAll(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, class := range []string{"temporary", "all"} {
		for _, format := range []string{"toon", "json"} {
			t.Run(class+"/"+format, func(t *testing.T) {
				args := []string{"workspace", "clean", "--workspace", "example", "--class", class}
				if format == "json" {
					args = append(args, "--json")
				}
				var output bytes.Buffer
				if code := app.Run(t.Context(), args, &output, options); code != 1 {
					t.Fatalf("exit=%d, output=%s", code, output.String())
				}
				for _, private := range []string{root, filepath.ToSlash(root), strings.Trim(strconv.Quote(root), `"`)} {
					if strings.Contains(output.String(), private) {
						t.Fatalf("rendered absolute workspace path %q: %s", private, output.String())
					}
				}
				if !strings.Contains(output.String(), "workspace.clean.failed") || !strings.Contains(output.String(), ".tadx/tmp.lock") {
					t.Fatalf("lost the cleanup error or relative lock path: %s", output.String())
				}
				for _, preserved := range []string{blocked, filepath.Join(root, "tadx.yaml")} {
					if _, err := os.Stat(preserved); err != nil {
						t.Fatalf("failed cleanup changed %q: %v", preserved, err)
					}
				}
			})
		}
	}
}
