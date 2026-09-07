package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestWorkspaceRemovalRequiresDefaultReassignmentThroughCLI(t *testing.T) {
	for _, operation := range []string{"unregister", "delete"} {
		t.Run(operation, func(t *testing.T) {
			directory := t.TempDir()
			root := filepath.Join(directory, "development")
			options := app.Options{ConfigPath: filepath.Join(directory, "config.yaml")}
			run := func(args ...string) (int, string) {
				var output bytes.Buffer
				exit := app.Run(context.Background(), args, &output, options)
				return exit, output.String()
			}
			if exit, out := run("workspace", "create", "development", "--path", root); exit != 0 {
				t.Fatalf("create: %s", out)
			}
			if exit, out := run("workspace", operation, "development"); exit == 0 || !strings.Contains(out, "default") {
				t.Fatalf("removal: exit=%d %s", exit, out)
			}
			if _, err := os.Stat(filepath.Join(root, "tadx.yaml")); err != nil {
				t.Fatal(err)
			}
			if exit, out := run("workspace", "create", "other", "--path", filepath.Join(directory, "other")); exit != 0 {
				t.Fatalf("create other: %s", out)
			}
			if exit, out := run("workspace", "set-default", "other"); exit != 0 {
				t.Fatalf("set default: %s", out)
			}
			if exit, out := run("workspace", operation, "development"); exit != 0 {
				t.Fatalf("removal after reassignment: %s", out)
			}
			_, statErr := os.Stat(root)
			if operation == "delete" && !os.IsNotExist(statErr) {
				t.Fatalf("deleted workspace exists: %v", statErr)
			}
			if operation == "unregister" && statErr != nil {
				t.Fatalf("unregistered files changed: %v", statErr)
			}
		})
	}
}
