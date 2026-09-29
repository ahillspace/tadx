package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// editInstalledPackages installs both packages for codex and then edits each
// one, so the next install must back up and replace both.
func editInstalledPackages(t *testing.T, home string) {
	t.Helper()
	in := Installer{Home: func() (string, error) { return home, nil }}
	if _, err := in.Install(context.Background(), "codex", false, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tadx", "tadx-pulse"} {
		location := filepath.Join(home, ".codex", "skills", name, "SKILL.md")
		if err := os.WriteFile(location, []byte("edited "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInstallReportsPackageStateWhenRollbackIsIncomplete(t *testing.T) {
	const tadxPath, pulsePath = ".codex/skills/tadx", ".codex/skills/tadx-pulse"
	commitsPulse := func(from, to string) bool { return to == pulsePath && strings.Contains(from, ".tadx-stage-") }
	for _, test := range []struct {
		name string
		// fail selects the rollback rename that fails after the pulse commit fails.
		fail       func(from, to string) bool
		wantStatus map[string]string
	}{
		{
			name:       "committed package cannot be withdrawn",
			fail:       func(from, to string) bool { return from == tadxPath && strings.Contains(to, ".tadx-stage-") },
			wantStatus: map[string]string{"tadx": "replaced", "tadx-pulse": "unchanged"},
		},
		{
			name:       "previous package cannot be restored",
			fail:       func(from, to string) bool { return to == pulsePath && strings.Contains(from, ".tadx-skill-backups") },
			wantStatus: map[string]string{"tadx": "replaced", "tadx-pulse": "backed-up"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			editInstalledPackages(t, home)
			in := Installer{Home: func() (string, error) { return home, nil }}
			in.rename = func(root *os.Root, from, to string) error {
				if commitsPulse(from, to) || test.fail(from, to) {
					return &os.LinkError{Op: "rename", Old: from, New: to, Err: errors.New("sharing violation")}
				}
				return root.Rename(from, to)
			}
			result, err := in.Install(context.Background(), "codex", false, false)
			if err == nil || !strings.Contains(err.Error(), "rollback is incomplete") {
				t.Fatalf("error = %v", err)
			}
			if result.Status != "partial" || len(result.Skills) != 2 {
				t.Fatalf("result = %#v", result)
			}
			for _, skill := range result.Skills {
				if skill.Status != test.wantStatus[skill.Name] {
					t.Fatalf("%s status = %q, want %q (%#v)", skill.Name, skill.Status, test.wantStatus[skill.Name], result)
				}
				if skill.Status == "unchanged" {
					if skill.Backup != "" {
						t.Fatalf("%s restored package still reports backup %q", skill.Name, skill.Backup)
					}
					data, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(skill.Path), "SKILL.md"))
					if err != nil || string(data) != "edited "+skill.Name {
						t.Fatalf("%s not restored: %q %v", skill.Name, data, err)
					}
					continue
				}
				// Every package left changed names where its previous version is kept.
				if strings.HasPrefix(skill.Backup, "/") || !strings.HasPrefix(skill.Backup, ".codex/.tadx-skill-backups/"+skill.Name+"-") {
					t.Fatalf("%s backup = %q", skill.Name, skill.Backup)
				}
				data, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(skill.Backup), "SKILL.md"))
				if err != nil || string(data) != "edited "+skill.Name {
					t.Fatalf("%s previous package not at its reported backup: %q %v", skill.Name, data, err)
				}
			}
		})
	}
}

func TestAutoInstallKeepsPackageStateOfATargetWithIncompleteRollback(t *testing.T) {
	home := t.TempDir()
	editInstalledPackages(t, home)
	in := Installer{Home: func() (string, error) { return home, nil }}
	in.rename = func(root *os.Root, from, to string) error {
		committingPulse := to == ".codex/skills/tadx-pulse" && strings.Contains(from, ".tadx-stage-")
		withdrawingTadx := from == ".codex/skills/tadx" && strings.Contains(to, ".tadx-stage-")
		if committingPulse || withdrawingTadx {
			return errors.New("sharing violation")
		}
		return root.Rename(from, to)
	}
	result, err := in.Install(context.Background(), "auto", false, false)
	if err == nil || result.Status != "partial" {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	got := map[string]string{}
	for _, skill := range result.Skills {
		if skill.Target != "codex" {
			t.Fatalf("skill without its target: %#v", skill)
		}
		if _, duplicate := got[skill.Name]; duplicate {
			t.Fatalf("duplicate %s entries: %#v", skill.Name, result.Skills)
		}
		got[skill.Name] = skill.Status
	}
	if got["tadx"] != "replaced" || got["tadx-pulse"] != "unchanged" || len(got) != 2 {
		t.Fatalf("statuses = %v", got)
	}
}

func TestInstallRemovesAbandonedStagingDirectories(t *testing.T) {
	home, outside := t.TempDir(), t.TempDir()
	base := filepath.Join(home, ".codex", "skills")
	abandoned := filepath.Join(base, ".tadx-stage-ABANDONED")
	if err := os.MkdirAll(filepath.Join(abandoned, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(abandoned, "nested", "SKILL.md"), []byte("name: tadx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, ".tadx-stage-FILE"), []byte("not a stage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := os.Symlink(outside, filepath.Join(base, ".tadx-stage-LINK")) == nil
	other := filepath.Join(base, "other-skill")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}

	in := Installer{Home: func() (string, error) { return home, nil }}
	result, err := in.Install(context.Background(), "codex", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(abandoned); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("abandoned stage remains: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("unrelated skill removed: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(outside, "keep")); err != nil || string(data) != "outside" {
		t.Fatalf("linked stage target changed: %q %v", data, err)
	}
	leftInPlace := []string{".tadx-stage-FILE"}
	if linked {
		leftInPlace = append(leftInPlace, ".tadx-stage-LINK")
	}
	for _, name := range leftInPlace {
		if _, err := os.Lstat(filepath.Join(base, name)); err != nil {
			t.Fatalf("%s removed: %v", name, err)
		}
		warned := false
		for _, warning := range result.Warnings {
			warned = warned || (strings.Contains(warning, name) && !strings.Contains(warning, home))
		}
		if !warned {
			t.Fatalf("no home-relative warning for %s: %v", name, result.Warnings)
		}
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tadx-stage-") && entry.Type().IsDir() {
			t.Fatalf("stage directory remains after install: %s", entry.Name())
		}
	}
}
