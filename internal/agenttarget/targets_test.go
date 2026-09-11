package agenttarget

import (
	"reflect"
	"testing"
)

func TestSupportedTargetsHaveDocumentedSkillRoots(t *testing.T) {
	want := []string{"claude", "cline", "codex", "copilot", "cursor", "gemini", "hermes", "opencode", "pi"}
	if got := SupportedTargets(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedTargets() = %q, want %q", got, want)
	}
	for target, wantPath := range map[string]string{
		"claude":   ".claude/skills",
		"cline":    ".cline/skills",
		"codex":    ".codex/skills",
		"copilot":  ".copilot/skills",
		"cursor":   ".cursor/skills",
		"gemini":   ".gemini/skills",
		"hermes":   ".hermes/skills",
		"opencode": ".config/opencode/skills",
		"pi":       ".pi/agent/skills",
	} {
		if got, ok := TargetPath(target); !ok || got != wantPath {
			t.Errorf("TargetPath(%q) = %q, %t; want %q, true", target, got, ok, wantPath)
		}
	}
	if got, ok := TargetPath("unknown"); ok || got != "" {
		t.Fatalf("TargetPath(unknown) = %q, %t", got, ok)
	}
}
