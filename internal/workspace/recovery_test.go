package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

func TestCreateRollbackPreservesOriginalRoot(t *testing.T) {
	for _, existed := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "empty"}[existed], func(t *testing.T) {
			directory := t.TempDir()
			root := filepath.Join(directory, "workspace")
			if existed {
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(directory, "config.yaml")
			if err := os.WriteFile(path, []byte("version: [invalid\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewManager(path, nil).Create(context.Background(), "test", root); err == nil {
				t.Fatal("malformed configuration accepted")
			}
			entries, err := os.ReadDir(root)
			if existed && (err != nil || len(entries) != 0) {
				t.Fatalf("original empty root not restored: %v %v", entries, err)
			}
			if !existed && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("created root was not rolled back: %v", err)
			}
		})
	}
}

func TestCreateRollbackPreservesConcurrentUnrelatedFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	finish, err := createRoot(root, Manifest{Version: 1, Workspace: ManifestWorkspace{ID: "ws_test", Name: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "artifacts", "notes.txt")
	if err := os.WriteFile(file, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := finish(true); err == nil {
		t.Fatal("expected preserved nonempty directory to be reported")
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "keep" {
		t.Fatalf("unrelated file lost: %q %v", data, err)
	}
}

func TestCreateRejectsSymlinkRoot(t *testing.T) {
	directory := t.TempDir()
	root := filepath.Join(directory, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := NewManager(filepath.Join(directory, "config.yaml"), nil).Create(context.Background(), "test", link); err == nil {
		t.Fatal("symlink root accepted")
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("symlink target changed: %v %v", entries, err)
	}
}

func TestReplacementGuidanceIsBoundedAndUsable(t *testing.T) {
	directory := t.TempDir()
	manager := NewManager(filepath.Join(directory, "config.yaml"), nil)
	for _, name := range []string{"target", "a", "b", "c", "d", "e", "f"} {
		if _, err := manager.Create(context.Background(), name, filepath.Join(directory, name)); err != nil {
			t.Fatal(err)
		}
	}
	_, err := manager.Unregister(context.Background(), "target")
	var recovery *RecoveryError
	if !errors.As(err, &recovery) || !strings.Contains(recovery.CorrectiveAction(), "tadx workspace set-default a") || !strings.Contains(recovery.CorrectiveAction(), "a, b, c, d, e;") {
		t.Fatalf("replacement advice: %v", err)
	}
	_, err = config.Update(manager.configPath, false, func(c config.Config) (config.Config, error) {
		c.DefaultWorkspace = "a"
		c.Environments = map[string]config.Environment{"dev": {URL: "https://example.test", Auth: config.Auth{Type: config.AuthTypePAT}, DefaultWorkspace: "target"}}
		return c, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Unregister(context.Background(), "target")
	if !errors.As(err, &recovery) || !strings.Contains(recovery.CorrectiveAction(), "tadx env update dev --default-workspace a") {
		t.Fatalf("environment replacement advice: %v", err)
	}
}
