package agent_test

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/agent"
	"github.com/ahillspace/tadx/internal/app"
	"gopkg.in/yaml.v3"
)

func TestInstalledSkillsIncludeAllBundledReferences(t *testing.T) {
	home := t.TempDir()
	installer := agent.Installer{Home: func() (string, error) { return home, nil }}
	if _, err := installer.Install(context.Background(), "codex", false, false); err != nil {
		t.Fatal(err)
	}
	source := os.DirFS("skills")
	err := fs.WalkDir(source, ".", func(location string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		expected, err := fs.ReadFile(source, location)
		if err != nil {
			return err
		}
		installed, err := os.ReadFile(filepath.Join(home, ".codex", "skills", filepath.FromSlash(location)))
		if err != nil {
			return err
		}
		if !bytes.Equal(expected, installed) {
			t.Errorf("installed package file differs from bundled source: %s", location)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCodexInstallPreservesLegacySkills(t *testing.T) {
	home := t.TempDir()
	legacyDirectory := filepath.Join(home, ".agents", "skills", "tadx")
	if err := os.MkdirAll(legacyDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyFile := filepath.Join(legacyDirectory, "SKILL.md")
	legacyContent := []byte("existing legacy skill with user changes")
	if err := os.WriteFile(legacyFile, legacyContent, 0o600); err != nil {
		t.Fatal(err)
	}
	installer := agent.Installer{Home: func() (string, error) { return home, nil }}
	if _, err := installer.Install(context.Background(), "codex", false, false); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(legacyFile)
	if err != nil || !bytes.Equal(actual, legacyContent) {
		t.Fatalf("legacy skill changed: %q, %v", actual, err)
	}
	for _, name := range []string{"tadx", "tadx-pulse"} {
		if _, err := os.Stat(filepath.Join(home, ".codex", "skills", name, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
}

// Validate complete source packages, including newly added optional references.
func TestSkillPackagesHaveValidMetadataAndLocalReferences(t *testing.T) {
	source := os.DirFS("skills")
	links := regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
	for _, name := range []string{"tadx", "tadx-pulse"} {
		entry, err := fs.ReadFile(source, name+"/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.SplitN(strings.ReplaceAll(string(entry), "\r\n", "\n"), "---\n", 3)
		if len(parts) != 3 || parts[0] != "" {
			t.Fatalf("%s has invalid frontmatter", name)
		}
		var metadata struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal([]byte(parts[1]), &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata.Name != name || strings.TrimSpace(metadata.Description) == "" {
			t.Fatalf("invalid metadata for %s: %#v", name, metadata)
		}
		err = fs.WalkDir(source, name, func(location string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(location, ".md") {
				return nil
			}
			data, err := fs.ReadFile(source, location)
			if err != nil {
				return err
			}
			for _, match := range links.FindAllStringSubmatch(string(data), -1) {
				if strings.Contains(match[1], "://") || strings.HasPrefix(match[1], "#") {
					continue
				}
				target := path.Clean(path.Join(path.Dir(location), strings.SplitN(match[1], "#", 2)[0]))
				if !strings.HasPrefix(target, name+"/") {
					t.Errorf("%s links outside its installed package: %s", location, target)
					continue
				}
				if _, err := fs.Stat(source, target); err != nil {
					t.Errorf("%s has missing reference %s: %v", location, target, err)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Help parsing validates published recipe command paths and flags without remote
// requests or mutation. Workflow behavior and latency need separate Luna trials.
func TestSkillRecipesUseInstalledCommandsAndFlags(t *testing.T) {
	source := os.DirFS("skills")
	words := regexp.MustCompile(`"[^"]*"|'[^']*'|[^\s]+`)
	options := app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}
	count := 0
	err := fs.WalkDir(source, ".", func(location string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(location, ".md") {
			return nil
		}
		data, err := fs.ReadFile(source, location)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "tadx ") {
				continue
			}
			count++
			args := words.FindAllString(line, -1)[1:]
			for index := range args {
				args[index] = strings.Trim(args[index], "\"'")
			}
			args = append(args, "--help")
			var out bytes.Buffer
			if exit := app.Run(context.Background(), args, &out, options); exit != 0 {
				t.Fatalf("%s recipe %q: exit %d\n%s", location, line, exit, out.String())
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no CLI recipes validated")
	}
	t.Logf("validated %d CLI recipes without executing operations", count)
}
