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
	for _, target := range []string{"codex", "claude", "cursor"} {
		t.Run(target, func(t *testing.T) {
			home := t.TempDir()
			installer := agent.Installer{Home: func() (string, error) { return home, nil }}
			if _, err := installer.Install(context.Background(), target, false, false); err != nil {
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
				installed, err := os.ReadFile(filepath.Join(home, "."+target, "skills", filepath.FromSlash(location)))
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
		})
	}
}

func TestCodexInstallRequiresForceAndBacksUpDivergentLegacySkills(t *testing.T) {
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
	if _, err := installer.Install(context.Background(), "codex", false, false); err == nil {
		t.Fatal("divergent legacy skill must require force")
	}
	actual, err := os.ReadFile(legacyFile)
	if err != nil || !bytes.Equal(actual, legacyContent) {
		t.Fatalf("legacy skill changed: %q, %v", actual, err)
	}
	result, err := installer.Install(context.Background(), "codex", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 3 || result.Skills[2].Backup == "" {
		t.Fatalf("missing legacy backup: %#v", result)
	}
	actual, err = os.ReadFile(filepath.Join(home, filepath.FromSlash(result.Skills[2].Backup), "SKILL.md"))
	if err != nil || !bytes.Equal(actual, legacyContent) {
		t.Fatalf("legacy backup changed: %q, %v", actual, err)
	}
	if _, err := os.Stat(legacyDirectory); !os.IsNotExist(err) {
		t.Fatalf("legacy skill remains discoverable: %v", err)
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
// requests or mutation. These checks do not validate agent workflow behavior.
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

func TestPulsePublishGuidanceFlagsAreExecutable(t *testing.T) {
	data, err := os.ReadFile("skills/tadx-pulse/references/operations.md")
	if err != nil {
		t.Fatal(err)
	}
	flags := regexp.MustCompile(`--[a-z][a-z-]*`)
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "| `tadx pulse definition publish` |") {
			continue
		}
		for _, flag := range flags.FindAllString(line, -1) {
			count++
			value := "example"
			if flag == "--preview" || flag == "--full" {
				value = "true"
			}
			var output bytes.Buffer
			args := []string{"pulse", "definition", "publish", flag + "=" + value, "--help"}
			if code := app.Run(context.Background(), args, &output, app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}); code != 0 {
				t.Errorf("documented publish flag %s is rejected: %s", flag, output.String())
			}
		}
	}
	if count == 0 {
		t.Fatal("no Pulse publish Guidance flags checked")
	}
}
