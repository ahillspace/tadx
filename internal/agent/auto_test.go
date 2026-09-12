package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAutoInstallDetectsTargetsPreservesOtherSkillsAndRefreshesOwned(t *testing.T) {
	home := t.TempDir()
	for _, base := range []string{".codex", ".config/opencode"} {
		other := filepath.Join(home, filepath.FromSlash(base), "skills", "my-skill")
		if err := os.MkdirAll(other, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(other, "SKILL.md"), []byte("untouched"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	in := Installer{Home: func() (string, error) { return home, nil }}
	preview, err := in.Install(context.Background(), "auto", true, false)
	if err != nil || !reflect.DeepEqual(preview.Targets, []string{"codex", "opencode"}) {
		t.Fatalf("preview: %#v %v", preview, err)
	}
	for _, base := range []string{".codex", ".config/opencode"} {
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(base), "skills", "tadx")); !os.IsNotExist(err) {
			t.Fatal("preview wrote package")
		}
	}
	result, err := in.Install(context.Background(), "auto", false, false)
	if err != nil || result.Status != "installed" || len(result.Skills) != 4 {
		t.Fatalf("install: %#v %v", result, err)
	}
	for _, base := range []string{".codex", ".config/opencode"} {
		if err := os.WriteFile(filepath.Join(home, filepath.FromSlash(base), "skills", "tadx", "SKILL.md"), []byte("edited"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err = in.Install(context.Background(), "auto", false, false)
	if err != nil || result.Status != "installed" {
		t.Fatalf("refresh: %#v %v", result, err)
	}
	for _, base := range []string{".codex", ".config/opencode"} {
		data, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(base), "skills", "my-skill", "SKILL.md"))
		if err != nil || string(data) != "untouched" {
			t.Fatalf("unrelated skill changed: %q %v", data, err)
		}
	}
}

func TestAutoInstallPortableFallbackAndPartialFailure(t *testing.T) {
	t.Run("fallback", func(t *testing.T) {
		home := t.TempDir()
		in := Installer{Home: func() (string, error) { return home, nil }}
		result, err := in.Install(context.Background(), "auto", false, false)
		if err != nil || !reflect.DeepEqual(result.Targets, []string{"generic"}) || len(result.Warnings) != 1 {
			t.Fatalf("fallback: %#v %v", result, err)
		}
		if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "tadx", "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("partial", func(t *testing.T) {
		home := t.TempDir()
		for _, base := range []string{".claude/skills", ".codex/skills"} {
			if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(base)), 0700); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(home, ".codex", "skills", ".tadx-install.lock"), []byte("busy"), 0600); err != nil {
			t.Fatal(err)
		}
		in := Installer{Home: func() (string, error) { return home, nil }}
		result, err := in.Install(context.Background(), "auto", false, false)
		if err == nil || result.Status != "partial" || len(result.Skills) != 3 || result.Skills[0].Target != "claude" || result.Skills[2].Status != "failed" {
			t.Fatalf("partial: %#v %v", result, err)
		}
	})
}

func TestPackageRenameFailurePreservesCauseAndRollsBack(t *testing.T) {
	home := t.TempDir()
	in := Installer{Home: func() (string, error) { return home, nil }}
	if _, err := in.Install(context.Background(), "codex", false, false); err != nil {
		t.Fatal(err)
	}
	location := filepath.Join(home, ".codex", "skills", "tadx", "SKILL.md")
	if err := os.WriteFile(location, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	cause := errors.New("The process cannot access the file because it is being used by another process")
	in.rename = func(_ *os.Root, from, to string) error {
		return &os.LinkError{Op: "rename", Old: filepath.Join(home, from), New: filepath.Join(home, to), Err: cause}
	}
	_, err := in.Install(context.Background(), "codex", false, false)
	if err == nil || !errors.Is(err, cause) || !strings.Contains(err.Error(), "close programs") || strings.Contains(err.Error(), home) {
		t.Fatalf("diagnostic: %v", err)
	}
	data, readErr := os.ReadFile(location)
	if readErr != nil || string(data) != "original" {
		t.Fatalf("original lost: %q %v", data, readErr)
	}
}

func TestCodexLeavesModernPortableInstallationIntact(t *testing.T) {
	home := t.TempDir()
	in := Installer{Home: func() (string, error) { return home, nil }}
	if _, err := in.Install(context.Background(), "generic", false, false); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"install", "uninstall"} {
		if _, err := runOperation(in, operation, false, false); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"tadx", "tadx-pulse"} {
			if _, err := os.Stat(filepath.Join(home, ".agents", "skills", name, "SKILL.md")); err != nil {
				t.Fatalf("portable %s removed by Codex %s: %v", name, operation, err)
			}
		}
	}
}

func TestAutoInstallRejectsLinkedAgentConfig(t *testing.T) {
	home, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, ".codex")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	in := Installer{Home: func() (string, error) { return home, nil }}
	if _, err := in.Install(context.Background(), "auto", false, false); err == nil {
		t.Fatal("auto followed agent link")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside target changed: %v %v", entries, err)
	}
}
