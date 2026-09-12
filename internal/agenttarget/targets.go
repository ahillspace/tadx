// Package agenttarget defines the global skill roots owned by supported agents.
package agenttarget

import "strings"

type target struct {
	name string
	path string
}

var targets = []target{
	{name: "claude", path: ".claude/skills"},
	{name: "cline", path: ".cline/skills"},
	{name: "codex", path: ".codex/skills"},
	{name: "copilot", path: ".copilot/skills"},
	{name: "cursor", path: ".cursor/skills"},
	{name: "gemini", path: ".gemini/skills"},
	{name: "generic", path: ".agents/skills"},
	{name: "hermes", path: ".hermes/skills"},
	{name: "opencode", path: ".config/opencode/skills"},
	{name: "pi", path: ".pi/agent/skills"},
}

// SupportedTargets returns the stable, supported target names.
func SupportedTargets() []string {
	names := make([]string, len(targets))
	for index, target := range targets {
		names[index] = target.name
	}
	return names
}

// TargetPath returns the home-relative global skill root for target.
func TargetPath(name string) (string, bool) {
	for _, target := range targets {
		if target.name == name {
			return target.path, true
		}
	}
	return "", false
}

// IsSupported reports whether target has a managed global skill root.
func IsSupported(target string) bool {
	_, ok := TargetPath(target)
	return ok
}

// Summary renders target names for bounded human-facing usage guidance.
func Summary() string {
	names := SupportedTargets()
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1]
}
