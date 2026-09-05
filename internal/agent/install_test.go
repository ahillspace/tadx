package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallPreflightsBothPackagesAndPreservesBackups(t *testing.T) {
	home := t.TempDir()
	installer := Installer{Home: func() (string, error) { return home, nil }}
	divergent := filepath.Join(home, ".codex", "skills", "tadx-pulse")
	if err := os.MkdirAll(divergent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(divergent, "SKILL.md"), []byte("custom pulse"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(divergent, "notes.txt"), []byte("keep notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(context.Background(), "codex", false, false); err == nil {
		t.Fatal("expected collision")
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "tadx")); !os.IsNotExist(err) {
		t.Fatalf("partial installation: %v", err)
	}
	preview, err := installer.Install(context.Background(), "codex", true, false)
	if err != nil || len(preview.Warnings) != 1 || preview.Skills[1].Status != "replace" {
		t.Fatalf("preview = %#v, %v", preview, err)
	}
	result, err := installer.Install(context.Background(), "codex", false, true)
	if err != nil {
		t.Fatal(err)
	}
	backup := result.Skills[1].Backup
	if !strings.HasPrefix(backup, ".codex/.tadx-skill-backups/") {
		t.Fatalf("backup = %q", backup)
	}
	for file, expected := range map[string]string{"SKILL.md": "custom pulse", "notes.txt": "keep notes"} {
		data, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(backup), file))
		if err != nil || string(data) != expected {
			t.Fatalf("backup %s = %q, %v", file, data, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(home, ".codex", "skills"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("staging/lock leftovers: %v, %v", entries, err)
	}
}

func TestInstallRejectsSymlinks(t *testing.T) {
	for _, location := range []string{".codex", ".codex/skills", ".codex/skills/tadx", ".codex/skills/tadx/references"} {
		t.Run(location, func(t *testing.T) {
			home, outside := t.TempDir(), t.TempDir()
			link := filepath.Join(home, filepath.FromSlash(location))
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Skipf("symbolic links unavailable: %v", err)
			}
			installer := Installer{Home: func() (string, error) { return home, nil }}
			if _, err := installer.Install(context.Background(), "codex", false, true); err == nil {
				t.Fatal("accepted symbolic link")
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatalf("outside changed: %v, %v", entries, err)
			}
		})
	}
}

func TestInstallRollsBackFirstPackageWhenSecondCommitFails(t *testing.T) {
	home := t.TempDir()
	base := filepath.Join(home, ".codex")
	pulse := filepath.Join(base, "skills", "tadx-pulse")
	if err := os.MkdirAll(pulse, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pulse, "SKILL.md"), []byte("original pulse"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A real filesystem collision prevents backup creation after the first commit.
	if err := os.WriteFile(filepath.Join(base, ".tadx-skill-backups"), []byte("keep this file"), 0o600); err != nil {
		t.Fatal(err)
	}
	installer := Installer{Home: func() (string, error) { return home, nil }}
	if _, err := installer.Install(context.Background(), "codex", false, true); err == nil {
		t.Fatal("expected backup collision")
	}
	if _, err := os.Stat(filepath.Join(base, "skills", "tadx")); !os.IsNotExist(err) {
		t.Fatalf("first commit was not rolled back: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(pulse, "SKILL.md"))
	if err != nil || string(data) != "original pulse" {
		t.Fatalf("original package changed: %q, %v", data, err)
	}
	entries, err := os.ReadDir(filepath.Join(base, "skills"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "tadx-pulse" {
		t.Fatalf("rollback left staging files: %v, %v", entries, err)
	}
}

func TestInstallRejectsCancellationAndConcurrentInstall(t *testing.T) {
	home := t.TempDir()
	installer := Installer{Home: func() (string, error) { return home, nil }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := installer.Install(ctx, "codex", false, false); err == nil {
		t.Fatal("accepted canceled context")
	}
	if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
		t.Fatalf("canceled install wrote files: %v", err)
	}
	base := filepath.Join(home, ".codex", "skills")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(base, ".tadx-install.lock")
	if err := os.WriteFile(lock, []byte("another installer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(context.Background(), "codex", false, true); err == nil {
		t.Fatal("ignored concurrent install")
	}
	if data, err := os.ReadFile(lock); err != nil || string(data) != "another installer" {
		t.Fatal("changed another install lock")
	}
}

func TestInstallRejectsNonDirectoryDestinationAndOversizedPackages(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "oversized"}[oversized], func(t *testing.T) {
			home := t.TempDir()
			base := filepath.Join(home, ".cursor", "skills")
			if err := os.MkdirAll(base, 0o755); err != nil {
				t.Fatal(err)
			}
			location := filepath.Join(base, "tadx")
			if oversized {
				if err := os.Mkdir(location, 0o755); err != nil {
					t.Fatal(err)
				}
				location = filepath.Join(location, "SKILL.md")
			}
			file, err := os.Create(location)
			if err != nil {
				t.Fatal(err)
			}
			if oversized {
				if err := file.Truncate(17 << 20); err != nil {
					t.Fatal(err)
				}
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			installer := Installer{Home: func() (string, error) { return home, nil }}
			if _, err := installer.Install(context.Background(), "cursor", false, true); err == nil {
				t.Fatal("accepted unsafe destination")
			}
		})
	}
}

func TestUninstallRemovesMatchingPackagesAndProtectsDivergentPackages(t *testing.T) {
	home := t.TempDir()
	installer := Installer{Home: func() (string, error) { return home, nil }}
	if _, err := installer.Install(context.Background(), "codex", false, false); err != nil {
		t.Fatal(err)
	}
	preview, err := installer.Uninstall(context.Background(), "codex", true, false)
	if err != nil || preview.Status != "preview" || preview.Skills[0].Status != "remove" {
		t.Fatalf("preview = %#v, %v", preview, err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "skills", "tadx", "custom.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Uninstall(context.Background(), "codex", false, false); err == nil {
		t.Fatal("uninstall accepted a divergent package without force")
	}
	result, err := installer.Uninstall(context.Background(), "codex", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "uninstalled" || result.Skills[0].Status != "backed-up" || result.Skills[0].Backup == "" || result.Skills[1].Status != "removed" {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "tadx")); !os.IsNotExist(err) {
		t.Fatalf("installed package remains: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(result.Skills[0].Backup), "custom.txt")); err != nil || string(data) != "keep" {
		t.Fatalf("backup content = %q, %v", data, err)
	}
}
