package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/config"
)

func TestWorkspaceRecoveryThroughCLI(t *testing.T) {
	for _, operation := range []string{"delete", "unregister"} {
		t.Run(operation, func(t *testing.T) {
			directory := t.TempDir()
			root := filepath.Join(directory, "existing-empty")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			options := app.Options{ConfigPath: filepath.Join(directory, "config.yaml")}
			run := func(args ...string) string {
				var output bytes.Buffer
				if exit := app.Run(context.Background(), args, &output, options); exit != 0 {
					t.Fatalf("%v: %s", args, output.String())
				}
				return output.String()
			}
			run("workspace", "create", "only", "--path", root)
			_, err := config.Update(options.ConfigPath, false, func(c config.Config) (config.Config, error) {
				c.Environments = map[string]config.Environment{"dev": {URL: "https://example.test", Auth: config.Auth{Type: config.AuthTypePAT}, DefaultWorkspace: "ONLY"}}
				return c, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			out := run("workspace", "clean", "--workspace", "only", "--class", "all")
			if !strings.Contains(out, "canonical_artifacts_preserved: true") || !strings.Contains(out, "entries_removed: 0") {
				t.Fatalf("cleanup receipt: %s", out)
			}
			if operation == "delete" {
				run("workspace", "delete", "only", "--preview")
				c, err := config.Load(options.ConfigPath)
				if err != nil || c.DefaultWorkspace != "only" || c.Environments["dev"].DefaultWorkspace != "ONLY" {
					t.Fatalf("preview changed defaults: %#v, %v", c, err)
				}
				if _, err := os.Stat(filepath.Join(root, "tadx.yaml")); err != nil {
					t.Fatalf("preview changed files: %v", err)
				}
			}
			run("workspace", operation, "only")
			c, err := config.Load(options.ConfigPath)
			if err != nil {
				t.Fatal(err)
			}
			if len(c.Workspaces) != 0 || c.DefaultWorkspace != "" || c.Environments["dev"].DefaultWorkspace != "" {
				t.Fatalf("stale workspace defaults: %#v", c)
			}
			_, err = os.Stat(filepath.Join(root, "tadx.yaml"))
			if operation == "unregister" && err != nil {
				t.Fatalf("unregister lost files: %v", err)
			}
			if operation == "delete" && !os.IsNotExist(err) {
				t.Fatalf("delete retained root: %v", err)
			}
		})
	}
}

func TestWorkspaceCreateRecoveryDoesNotAdoptOrdinaryFilesThroughCLI(t *testing.T) {
	directory := t.TempDir()
	root := filepath.Join(directory, "ordinary")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(file, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	exit := app.Run(context.Background(), []string{"workspace", "create", "ordinary", "--path", root}, &output, app.Options{ConfigPath: filepath.Join(directory, "config.yaml")})
	if exit == 0 || !strings.Contains(output.String(), "nonempty") || strings.Contains(output.String(), "workspace register") {
		t.Fatalf("ordinary directory guidance: %s", output.String())
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "keep" {
		t.Fatalf("ordinary file changed: %q, %v", data, err)
	}
}

func TestWorkspaceRegisterRecoveryClassifiesRootThroughCLI(t *testing.T) {
	for _, state := range []string{"missing", "empty", "nonempty", "managed"} {
		t.Run(state, func(t *testing.T) {
			directory := t.TempDir()
			root := filepath.Join(directory, "workspace")
			options := app.Options{ConfigPath: filepath.Join(directory, "config.yaml")}
			run := func(args ...string) (int, string) {
				var output bytes.Buffer
				exit := app.Run(context.Background(), args, &output, options)
				return exit, output.String()
			}
			if state != "missing" {
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if state == "nonempty" {
				if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if state == "managed" {
				if exit, out := run("workspace", "create", "test", "--path", root); exit != 0 {
					t.Fatal(out)
				}
				if exit, out := run("workspace", "unregister", "test"); exit != 0 {
					t.Fatal(out)
				}
				if exit, out := run("workspace", "create", "test", "--path", root); exit == 0 || !strings.Contains(out, "workspace register test --path") {
					t.Fatalf("managed recovery: %s", out)
				}
				if exit, out := run("workspace", "register", "test", "--path", root); exit != 0 {
					t.Fatalf("managed register: %s", out)
				}
				return
			}
			if exit, out := run("workspace", "register", "test", "--path", root); exit == 0 || !strings.Contains(out, state) || !strings.Contains(out, "workspace create test --path") {
				t.Fatalf("%s root recovery: %s", state, out)
			}
		})
	}
}
