package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func seedOldInstalledSkill(t *testing.T, home, target string) string {
	t.Helper()
	directory := filepath.Join(home, "."+target, "skills", "tadx")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("untouched previous bundled release"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	digest, err := fingerprint(root, "."+target+"/skills/tadx")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"version": 1, "packages": map[string]string{"tadx": digest}})
	if err := os.WriteFile(filepath.Join(home, "."+target, ".tadx-skill-receipt.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestIdenticalInstallSeedsReceiptOutsideSkillDiscovery(t *testing.T) {
	home := t.TempDir()
	installer := Installer{Home: func() (string, error) { return home, nil }}
	if _, err := installer.Install(context.Background(), "codex", false, false); err != nil {
		t.Fatal(err)
	}
	location := filepath.Join(home, ".codex", ".tadx-skill-receipt.json")
	if err := os.Remove(location); err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(context.Background(), "codex", true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(location); !os.IsNotExist(err) {
		t.Fatal("preview wrote a receipt")
	}
	result, err := installer.Install(context.Background(), "codex", false, false)
	if err != nil || result.Status != "unchanged" {
		t.Fatalf("identical install: %#v %v", result, err)
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	receipt, err := readReceipt(root, ".codex/skills")
	if err != nil || len(receipt.Packages) != 2 {
		t.Fatalf("receipt: %#v %v", receipt, err)
	}
	for name, digest := range receipt.Packages {
		installed, err := fingerprint(root, ".codex/skills/"+name)
		if err != nil || installed != digest {
			t.Fatalf("receipt does not match %s", name)
		}
	}
	entries, err := os.ReadDir(filepath.Join(home, ".codex", "skills"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("receipt leaked into skill discovery: %v %v", entries, err)
	}
}

func TestReceiptCommitFailureRollsBackInstallAndUninstall(t *testing.T) {
	for _, operation := range []string{"install", "uninstall", "force-edited"} {
		t.Run(operation, func(t *testing.T) {
			home := t.TempDir()
			seedOldInstalledSkill(t, home, "codex")
			if operation == "force-edited" {
				if err := os.WriteFile(filepath.Join(home, ".codex", "skills", "tadx", "notes.txt"), []byte("keep local change"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			root, err := os.OpenRoot(home)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			before, err := fingerprint(root, ".codex/skills/tadx")
			if err != nil {
				t.Fatal(err)
			}
			receiptBefore, err := root.ReadFile(".codex/.tadx-skill-receipt.json")
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			installer := Installer{Home: func() (string, error) { return home, nil }, commitReceipt: func(root *os.Root, from, to string) error {
				calls++
				if _, err := root.Stat(".codex/skills/.tadx-install.lock"); err != nil {
					t.Error("receipt commit occurred without package lock")
				}
				return errors.New("injected atomic receipt failure")
			}}
			if operation == "uninstall" {
				_, err = installer.Uninstall(context.Background(), "codex", false, false)
			} else {
				_, err = installer.Install(context.Background(), "codex", false, operation == "force-edited")
			}
			if err == nil || calls != 1 {
				t.Fatalf("failure seam calls=%d err=%v", calls, err)
			}
			after, err := fingerprint(root, ".codex/skills/tadx")
			if err != nil || before != after {
				t.Fatalf("package not restored: %s %s %v", before, after, err)
			}
			if _, err := root.Stat(".codex/skills/tadx-pulse"); !os.IsNotExist(err) {
				t.Fatalf("new package survived failed transaction: %v", err)
			}
			receiptAfter, err := root.ReadFile(".codex/.tadx-skill-receipt.json")
			if err != nil || !bytes.Equal(receiptBefore, receiptAfter) {
				t.Fatal("failed transaction changed ownership receipt")
			}
			entries, err := os.ReadDir(filepath.Join(home, ".codex", "skills"))
			if err != nil || len(entries) != 1 || entries[0].Name() != "tadx" {
				t.Fatalf("transaction left discovery debris: %v %v", entries, err)
			}
		})
	}
}

func TestInstallUpgradesUntouchedPreviousBundleWithoutForce(t *testing.T) {
	for _, target := range []string{"codex", "claude", "cursor"} {
		t.Run(target, func(t *testing.T) {
			home := t.TempDir()
			seedOldInstalledSkill(t, home, target)
			installer := Installer{Home: func() (string, error) { return home, nil }}
			if _, err := installer.Install(context.Background(), target, false, false); err != nil {
				t.Fatalf("untouched upgrade requires force: %v", err)
			}
			if _, err := installer.Uninstall(context.Background(), target, false, false); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUninstallRecognizesUntouchedPreviousBundleReceipt(t *testing.T) {
	home := t.TempDir()
	seedOldInstalledSkill(t, home, "codex")
	installer := Installer{Home: func() (string, error) { return home, nil }}
	if _, err := installer.Uninstall(context.Background(), "codex", false, false); err != nil {
		t.Fatalf("untouched old uninstall requires force: %v", err)
	}
}

func TestLocallyEditedOwnedPackageRefreshesWithoutForce(t *testing.T) {
	home := t.TempDir()
	seedOldInstalledSkill(t, home, "codex")
	if err := os.WriteFile(filepath.Join(home, ".codex", "skills", "tadx", "notes.txt"), []byte("local edits"), 0o600); err != nil {
		t.Fatal(err)
	}
	installer := Installer{Home: func() (string, error) { return home, nil }}
	result, err := installer.Install(context.Background(), "codex", false, false)
	if err != nil || result.Skills[0].Backup == "" {
		t.Fatalf("forced replacement omitted backup: %#v %v", result, err)
	}
}
