package guidancenotice

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestConcurrentLaunchesClaimOneNotice(t *testing.T) {
	options := testOptions(t, t.TempDir())
	var count atomic.Int32
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			if ShouldPrint(options) {
				count.Add(1)
			}
		})
	}
	group.Wait()
	if got := count.Load(); got != 1 {
		t.Fatalf("notice claims=%d, want exactly one", got)
	}
}

func TestShouldPrintOncePerSessionWhenGuidanceIsMissing(t *testing.T) {
	directory := t.TempDir()
	options := testOptions(t, directory)
	if !ShouldPrint(options) {
		t.Fatal("first launch should print a notice")
	}
	if ShouldPrint(options) {
		t.Fatal("second launch in the same session should not print a notice")
	}
	options.Session = func() string { return "new-session" }
	if !ShouldPrint(options) {
		t.Fatal("a new session should print a notice")
	}
}

func TestShouldPrintStopsWhenRootGuidanceExists(t *testing.T) {
	for _, root := range []string{
		".cline/skills",
		".codex/skills",
		".copilot/skills",
		".gemini/skills",
	} {
		t.Run(root, func(t *testing.T) {
			directory := t.TempDir()
			options := testOptions(t, directory)
			writeSkill(t, filepath.Join(directory, "home", filepath.FromSlash(root), "tadx", "SKILL.md"))
			if ShouldPrint(options) {
				t.Fatal("installed root guidance should suppress the notice")
			}
		})
	}
}

func TestShouldPrintDoesNotRequirePulseGuidance(t *testing.T) {
	directory := t.TempDir()
	options := testOptions(t, directory)
	writeSkill(t, filepath.Join(directory, "home", ".codex", "skills", "tadx-pulse", "SKILL.md"))
	if !ShouldPrint(options) {
		t.Fatal("pulse guidance alone should not suppress the root guidance notice")
	}
}

func TestShouldPrintChecksProjectAndLegacyRoots(t *testing.T) {
	for _, location := range []string{
		filepath.Join("project", ".github", "skills", "tadx", "SKILL.md"),
		filepath.Join("project", ".gemini", "skills", "tadx", "SKILL.md"),
		filepath.Join("project", ".cline", "skills", "tadx", "SKILL.md"),
		filepath.Join("project", ".clinerules", "skills", "tadx", "SKILL.md"),
		filepath.Join("project", ".agents", "skills", "tadx", "SKILL.md"),
		filepath.Join("home", ".agents", "skills", "tadx", "SKILL.md"),
	} {
		t.Run(location, func(t *testing.T) {
			directory := t.TempDir()
			options := testOptions(t, directory)
			writeSkill(t, filepath.Join(directory, location))
			if ShouldPrint(options) {
				t.Fatal("discoverable root guidance should suppress the notice")
			}
		})
	}
}

func TestShouldPrintChecksConfiguredSkillRoots(t *testing.T) {
	directory := t.TempDir()
	options := testOptions(t, directory)
	configured := filepath.Join(directory, "configured-codex")
	writeSkill(t, filepath.Join(configured, "skills", "tadx", "SKILL.md"))
	options.Env = func(name string) (string, bool) {
		return configured, name == "CODEX_HOME"
	}
	if ShouldPrint(options) {
		t.Fatal("configured Codex guidance should suppress the notice")
	}
}

func TestShouldPrintSuppressesOptOutAndCompletion(t *testing.T) {
	for _, args := range [][]string{{"completion", "bash"}, {"cmp", "bash"}, {"__complete"}} {
		directory := t.TempDir()
		options := testOptions(t, directory)
		options.Args = args
		if ShouldPrint(options) {
			t.Fatalf("args %q should not print", args)
		}
	}
	directory := t.TempDir()
	options := testOptions(t, directory)
	options.Env = func(name string) (string, bool) { return "0", name == "TADX_GUIDANCE_NOTICE" }
	if ShouldPrint(options) {
		t.Fatal("TADX_GUIDANCE_NOTICE=0 should suppress the notice")
	}
}

func TestShouldPrintFailsSilentWhenCacheCannotBeWritten(t *testing.T) {
	options := testOptions(t, t.TempDir())
	options.MkdirAll = func(string, os.FileMode) error { return errors.New("read only") }
	if ShouldPrint(options) {
		t.Fatal("an inaccessible cache should fail silent")
	}
}

func testOptions(t *testing.T, directory string) Options {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(directory, "project", ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	return Options{
		Args:    []string{"search", "sales"},
		Env:     func(string) (string, bool) { return "", false },
		Home:    func() (string, error) { return filepath.Join(directory, "home"), nil },
		WorkDir: func() (string, error) { return filepath.Join(directory, "project"), nil },
		Cache:   func() (string, error) { return filepath.Join(directory, "cache"), nil },
		Session: func() string { return "same-session" },
	}
}

func TestOverviewNeverWritesNoticeMarker(t *testing.T) {
	for _, args := range [][]string{nil, {"--full"}, {"--json"}, {"--config", "config.yaml", "--full"}, {"--cfg=config.yaml", "-f"}} {
		options := testOptions(t, t.TempDir())
		options.Args = args
		options.MkdirAll = func(string, os.FileMode) error { t.Fatal("overview attempted a notice write"); return nil }
		if ShouldPrint(options) {
			t.Fatalf("overview %v should not emit a notice", args)
		}
	}
}

func writeSkill(t *testing.T, filename string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte("placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
}
