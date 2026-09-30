package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUninstallReportsPackageStateWhenRollbackIsIncomplete(t *testing.T) {
	const tadxPath, pulsePath = ".codex/skills/tadx", ".codex/skills/tadx-pulse"
	home := t.TempDir()
	if _, err := (Installer{Home: func() (string, error) { return home, nil }}).Install(context.Background(), "codex", false, false); err != nil {
		t.Fatal(err)
	}
	in := Installer{Home: func() (string, error) { return home, nil }}
	in.rename = func(root *os.Root, from, to string) error {
		// Staging the pulse package fails, and restoring the already staged
		// tadx package fails too, so the rollback stops with tadx staged.
		if from == pulsePath && strings.Contains(to, ".tadx-skill-staging") || to == tadxPath && strings.Contains(from, ".tadx-skill-staging") {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: errors.New("sharing violation")}
		}
		return root.Rename(from, to)
	}
	result, err := in.Uninstall(context.Background(), "codex", false, false)
	if err == nil || !strings.Contains(err.Error(), "rollback is incomplete") {
		t.Fatalf("error = %v", err)
	}
	if result.Status != "partial" || len(result.Skills) != 2 {
		t.Fatalf("result = %#v", result)
	}
	want := map[string]string{"tadx": "backed-up", "tadx-pulse": "unchanged"}
	for _, skill := range result.Skills {
		if skill.Status != want[skill.Name] {
			t.Fatalf("%s status = %q, want %q (%#v)", skill.Name, skill.Status, want[skill.Name], result)
		}
		if skill.Status == "unchanged" {
			if skill.Backup != "" {
				t.Fatalf("%s package still in place reports backup %q", skill.Name, skill.Backup)
			}
			if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(skill.Path), "SKILL.md")); err != nil {
				t.Fatalf("%s not in place: %v", skill.Name, err)
			}
			continue
		}
		// A package left out of discovery names where its files are kept.
		if strings.HasPrefix(skill.Backup, "/") || !strings.HasPrefix(skill.Backup, ".codex/.tadx-skill-staging/"+skill.Name+"-") {
			t.Fatalf("%s backup = %q", skill.Name, skill.Backup)
		}
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(skill.Backup), "SKILL.md")); err != nil {
			t.Fatalf("%s package not at its reported backup: %v", skill.Name, err)
		}
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(skill.Path))); !os.IsNotExist(err) {
			t.Fatalf("%s reported as backed up but still discoverable: %v", skill.Name, err)
		}
	}
}

func TestUninstallRollbackThatCompletesReturnsOnlyTheCause(t *testing.T) {
	const pulsePath = ".codex/skills/tadx-pulse"
	home := t.TempDir()
	if _, err := (Installer{Home: func() (string, error) { return home, nil }}).Install(context.Background(), "codex", false, false); err != nil {
		t.Fatal(err)
	}
	in := Installer{Home: func() (string, error) { return home, nil }}
	in.rename = func(root *os.Root, from, to string) error {
		if from == pulsePath && strings.Contains(to, ".tadx-skill-staging") {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: errors.New("sharing violation")}
		}
		return root.Rename(from, to)
	}
	result, err := in.Uninstall(context.Background(), "codex", false, false)
	if err == nil || strings.Contains(err.Error(), "rollback is incomplete") {
		t.Fatalf("error = %v", err)
	}
	if result.Status != "" || result.Skills != nil {
		t.Fatalf("restored uninstall reported package state: %#v", result)
	}
	for _, name := range []string{"tadx", "tadx-pulse"} {
		if _, err := os.Stat(filepath.Join(home, ".codex", "skills", name, "SKILL.md")); err != nil {
			t.Fatalf("%s not restored: %v", name, err)
		}
	}
}
