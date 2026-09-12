package agent

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestBundle(t *testing.T, home, base, name string) {
	t.Helper()
	files, err := readBundle(name)
	if err != nil {
		t.Fatal(err)
	}
	for relative, data := range files {
		location := filepath.Join(home, filepath.FromSlash(path.Join(base, name, relative)))
		if err := os.MkdirAll(filepath.Dir(location), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(location, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func runOperation(in Installer, operation string, preview, force bool) (Result, error) {
	if operation == "install" {
		return in.Install(context.Background(), "codex", preview, force)
	}
	return in.Uninstall(context.Background(), "codex", preview, force)
}

func TestLegacyPreviewAndForcePreserveDivergentPackages(t *testing.T) {
	for _, operation := range []string{"install", "uninstall"} {
		t.Run(operation, func(t *testing.T) {
			home := t.TempDir()
			in := Installer{Home: func() (string, error) { return home, nil }}
			writeTestBundle(t, home, legacyCodexBase, "tadx")
			writeTestBundle(t, home, legacyCodexBase, "tadx-pulse")
			notes := filepath.Join(home, ".agents", "skills", "tadx-pulse", "notes.txt")
			if err := os.WriteFile(notes, []byte("preserve customization"), 0o600); err != nil {
				t.Fatal(err)
			}
			preview, err := runOperation(in, operation, true, false)
			if err != nil || preview.Status != "preview" || len(preview.Skills) != 4 || len(preview.Warnings) != 0 {
				t.Fatalf("preview = %#v, %v", preview, err)
			}
			if preview.Skills[2].Status != "remove" || preview.Skills[3].Status != "divergent" {
				t.Fatalf("legacy preview = %#v", preview.Skills)
			}
			if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
				t.Fatalf("preview or rejected operation wrote canonical directory: %v", err)
			}
			result, err := runOperation(in, operation, false, false)
			if err != nil {
				t.Fatal(err)
			}
			if result.Skills[3].Status != "backed-up" || !strings.HasPrefix(result.Skills[3].Backup, ".agents/.tadx-skill-backups/") {
				t.Fatalf("legacy backup = %#v", result)
			}
			data, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(result.Skills[3].Backup), "notes.txt"))
			if err != nil || string(data) != "preserve customization" {
				t.Fatalf("backup = %q, %v", data, err)
			}
			for _, name := range []string{"tadx", "tadx-pulse"} {
				if _, err := os.Stat(filepath.Join(home, ".agents", "skills", name)); !os.IsNotExist(err) {
					t.Fatalf("legacy %s still discoverable: %v", name, err)
				}
			}
		})
	}
}

func TestLegacyRemovalFailureRollsBackBothRoots(t *testing.T) {
	for _, operation := range []string{"install", "uninstall"} {
		t.Run(operation, func(t *testing.T) {
			home := t.TempDir()
			in := Installer{Home: func() (string, error) { return home, nil }}
			for _, name := range []string{"tadx", "tadx-pulse"} {
				writeTestBundle(t, home, legacyCodexBase, name)
				if operation == "uninstall" {
					writeTestBundle(t, home, ".codex/skills", name)
				}
			}
			if err := os.WriteFile(filepath.Join(home, ".agents", "skills", "tadx-pulse", "notes.txt"), []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			// The last package cannot move to its backup directory. Earlier
			// canonical commits and legacy removals must all roll back.
			if err := os.WriteFile(filepath.Join(home, ".agents", ".tadx-skill-backups"), []byte("collision"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := runOperation(in, operation, false, true); err == nil || strings.Contains(err.Error(), "rollback is incomplete") {
				t.Fatalf("expected a recoverable staging failure: %v", err)
			}
			for _, name := range []string{"tadx", "tadx-pulse"} {
				if _, err := os.Stat(filepath.Join(home, ".agents", "skills", name, "SKILL.md")); err != nil {
					t.Fatalf("legacy %s not restored: %v", name, err)
				}
				_, err := os.Stat(filepath.Join(home, ".codex", "skills", name, "SKILL.md"))
				if operation == "uninstall" && err != nil || operation == "install" && !os.IsNotExist(err) {
					t.Fatalf("canonical %s not restored: %v", name, err)
				}
			}
		})
	}
}

func TestUninstallCleanupFailureIsSuccessfulAndOutsideDiscovery(t *testing.T) {
	for _, target := range []string{"claude", "codex", "cursor"} {
		t.Run(target, func(t *testing.T) {
			home := t.TempDir()
			in := Installer{Home: func() (string, error) { return home, nil }}
			if _, err := in.Install(context.Background(), target, false, false); err != nil {
				t.Fatal(err)
			}
			if target == "codex" {
				writeTestBundle(t, home, legacyCodexBase, "tadx")
				writeTestBundle(t, home, legacyCodexBase, "tadx-pulse")
			}
			calls := 0
			in.removeAll = func(root *os.Root, location string) error {
				calls++
				if strings.Contains(location, "/skills/") || filepath.IsAbs(location) || strings.Contains(location, "\\") {
					t.Fatalf("unsafe removal stage: %q", location)
				}
				if _, err := root.Stat(path.Join(location, "SKILL.md")); err != nil {
					t.Fatalf("package not staged before cleanup: %v", err)
				}
				return errors.New("simulated cleanup failure")
			}
			result, err := in.Uninstall(context.Background(), target, false, false)
			if err != nil || result.Status != "uninstalled" || calls != len(result.Skills) || len(result.Warnings) != calls {
				t.Fatalf("cleanup outcome = %#v, %v; calls = %d", result, err, calls)
			}
			for _, skill := range result.Skills {
				if skill.Status != "removed" || skill.Backup == "" {
					t.Fatalf("cleanup result = %#v", skill)
				}
				if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(skill.Path))); !os.IsNotExist(err) {
					t.Fatalf("removed skill still discoverable: %v", err)
				}
			}
		})
	}
}

func TestLegacyLockBlocksBothOperations(t *testing.T) {
	for _, operation := range []string{"install", "uninstall"} {
		t.Run(operation, func(t *testing.T) {
			home := t.TempDir()
			writeTestBundle(t, home, legacyCodexBase, "tadx")
			lock := filepath.Join(home, ".agents", "skills", ".tadx-install.lock")
			if err := os.WriteFile(lock, []byte("owned by another installer"), 0o600); err != nil {
				t.Fatal(err)
			}
			in := Installer{Home: func() (string, error) { return home, nil }}
			if _, err := runOperation(in, operation, false, true); err == nil || !strings.Contains(err.Error(), "lock") {
				t.Fatalf("legacy lock ignored: %v", err)
			}
			if data, err := os.ReadFile(lock); err != nil || string(data) != "owned by another installer" {
				t.Fatalf("legacy lock changed: %q, %v", data, err)
			}
			if _, err := os.Stat(filepath.Join(home, ".codex", "skills", ".tadx-install.lock")); !os.IsNotExist(err) {
				t.Fatalf("canonical lock retained after rejection: %v", err)
			}
		})
	}
}

func TestLegacySymlinksAreRejected(t *testing.T) {
	for _, location := range []string{".agents", ".agents/skills", ".agents/skills/tadx", ".agents/skills/tadx/references"} {
		t.Run(location, func(t *testing.T) {
			home, outside := t.TempDir(), t.TempDir()
			link := filepath.Join(home, filepath.FromSlash(location))
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Skipf("symbolic links unavailable: %v", err)
			}
			in := Installer{Home: func() (string, error) { return home, nil }}
			for _, operation := range []string{"install", "uninstall"} {
				if _, err := runOperation(in, operation, false, true); err == nil {
					t.Fatalf("%s accepted a legacy symbolic link", operation)
				}
			}
			if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
				t.Fatalf("outside changed: %v, %v", entries, err)
			}
		})
	}
}

func TestInstallCleansLegacyDuplicatesWhenCanonicalPackagesAreUnchanged(t *testing.T) {
	home := t.TempDir()
	in := Installer{Home: func() (string, error) { return home, nil }}
	if _, err := in.Install(context.Background(), "codex", false, false); err != nil {
		t.Fatal(err)
	}
	writeTestBundle(t, home, legacyCodexBase, "tadx")
	unrelated := filepath.Join(home, ".agents", "skills", "other-skill")
	if err := os.MkdirAll(unrelated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unrelated, "SKILL.md"), []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	in.removeAll = func(_ *os.Root, location string) error {
		if !strings.HasPrefix(location, ".agents/.tadx-skill-staging/") {
			t.Fatalf("legacy cleanup occurs in discovery: %s", location)
		}
		return errors.New("simulated cleanup failure")
	}
	result, err := in.Install(context.Background(), "codex", false, false)
	if err != nil || result.Status != "installed" || len(result.Skills) != 3 || len(result.Warnings) != 1 {
		t.Fatalf("migration result = %#v, %v", result, err)
	}
	if result.Skills[0].Status != "unchanged" || result.Skills[1].Status != "unchanged" || result.Skills[2].Status != "removed" {
		t.Fatalf("unexpected migration states: %#v", result.Skills)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "tadx")); !os.IsNotExist(err) {
		t.Fatalf("legacy duplicate remains: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(unrelated, "SKILL.md")); err != nil || string(data) != "unrelated" {
		t.Fatalf("unrelated package changed: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(result.Skills[2].Backup), "SKILL.md")); err != nil {
		t.Fatalf("cleanup warning omitted recoverable staged package: %v", err)
	}
}
