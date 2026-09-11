package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeInstructionsImportRepositoryGuidance(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, "CLAUDE.md")
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("read CLAUDE.md file information: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatal("CLAUDE.md must be a regular file so its import works across platforms")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if got := strings.TrimSpace(string(content)); got != "@AGENTS.md" {
		t.Fatalf("CLAUDE.md = %q, want the single import %q", got, "@AGENTS.md")
	}
	guidance, err := os.Stat(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read imported AGENTS.md file information: %v", err)
	}
	if !guidance.Mode().IsRegular() {
		t.Fatal("imported AGENTS.md must resolve to a regular file")
	}
}
