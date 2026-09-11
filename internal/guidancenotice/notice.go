// Package guidancenotice provides a best-effort, once-per-session onboarding notice.
package guidancenotice

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahillspace/tadx/internal/agenttarget"
)

// Message includes supported targets so onboarding needs no help lookup.
func Message() string {
	return "TADX Guidance was not detected. For AI-assisted workflows, install it with `tadx agent install --target <" + strings.Join(agenttarget.SupportedTargets(), "|") + ">`. Set TADX_GUIDANCE_NOTICE=0 to suppress this notice.\n"
}

// Target identifies a skill directory relative to either the user home or the
// current project. It is deliberately metadata-only so startup does not load
// agent packages or their skill contents.
type Target struct {
	Name  string
	Roots []string
}

// Options supplies the small set of filesystem and environment dependencies
// used by ShouldPrint. Nil functions use the process defaults.
type Options struct {
	Args     []string
	Targets  []Target
	Env      func(string) (string, bool)
	Home     func() (string, error)
	WorkDir  func() (string, error)
	Cache    func() (string, error)
	Session  func() string
	Stat     func(string) (fs.FileInfo, error)
	MkdirAll func(string, fs.FileMode) error
	OpenFile func(string, int, fs.FileMode) (io.Closer, error)
}

// ShouldPrint reports whether a missing-guidance notice should be emitted.
// It never returns an error: onboarding must not affect the requested command.
func ShouldPrint(options Options) bool {
	if suppressed(options.Args) || disabled(options.Env) || hasGuidance(options) {
		return false
	}
	session := sessionID(options)
	if session == "" {
		return false
	}
	cache, err := cacheDir(options)
	if err != nil || cache == "" {
		return false
	}
	directory := filepath.Join(cache, "tadx", "guidance-notice")
	if err := mkdirAll(options, directory, 0o700); err != nil {
		return false
	}
	name := hash(session) + ".seen"
	marker, err := openFile(options, filepath.Join(directory, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false
	}
	_ = marker.Close()
	return true
}

func disabled(lookup func(string) (string, bool)) bool {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	value, ok := lookup("TADX_GUIDANCE_NOTICE")
	return ok && strings.TrimSpace(value) == "0"
}

func suppressed(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return args[0] == "completion" || args[0] == "cmp" || strings.HasPrefix(args[0], "__complete")
}

func hasGuidance(options Options) bool {
	home, homeErr := homeDir(options)
	workDir, workErr := workDir(options)
	targets := options.Targets
	if targets == nil {
		targets = supportedTargets()
	}
	for _, target := range targets {
		for _, root := range target.Roots {
			if homeErr == nil && skillExists(options, filepath.Join(home, filepath.FromSlash(root), "tadx", "SKILL.md")) {
				return true
			}
		}
	}
	if hasOverrideGuidance(options) {
		return true
	}
	if workErr == nil {
		for _, project := range projectRoots(options, workDir) {
			if skillExists(options, filepath.Join(project, "tadx", "SKILL.md")) {
				return true
			}
		}
	}
	return false
}

func supportedTargets() []Target {
	targets := make([]Target, 0, len(agenttarget.SupportedTargets()))
	for _, name := range agenttarget.SupportedTargets() {
		if root, ok := agenttarget.TargetPath(name); ok {
			targets = append(targets, Target{Name: name, Roots: []string{root}})
		}
	}
	return append(targets, Target{Name: "shared", Roots: []string{".agents/skills"}})
}

func hasOverrideGuidance(options Options) bool {
	lookup := options.Env
	if lookup == nil {
		lookup = os.LookupEnv
	}
	for name, suffix := range map[string]string{
		"CODEX_HOME":          "skills",
		"XDG_CONFIG_HOME":     "opencode/skills",
		"PI_CODING_AGENT_DIR": "skills",
		"HERMES_HOME":         "skills",
	} {
		if root, ok := lookup(name); ok && strings.TrimSpace(root) != "" && skillExists(options, filepath.Join(root, filepath.FromSlash(suffix), "tadx", "SKILL.md")) {
			return true
		}
	}
	return false
}

func projectRoots(options Options, directory string) []string {
	var roots []string
	for index := 0; index < 8; index++ {
		for _, root := range []string{".agents/skills", ".claude/skills", ".cline/skills", ".clinerules/skills", ".codex/skills", ".gemini/skills", ".github/skills", ".cursor/skills", ".opencode/skills", ".pi/skills"} {
			roots = append(roots, filepath.Join(directory, filepath.FromSlash(root)))
		}
		if _, err := stat(options, filepath.Join(directory, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
	}
	return roots
}

func skillExists(options Options, filename string) bool {
	info, err := stat(options, filename)
	return err == nil && !info.IsDir() && info.Mode().IsRegular()
}

func sessionID(options Options) string {
	if options.Session != nil {
		return options.Session()
	}
	lookup := options.Env
	if lookup == nil {
		lookup = os.LookupEnv
	}
	if value, ok := lookup("TADX_GUIDANCE_SESSION"); ok && strings.TrimSpace(value) != "" {
		return "override:" + value
	}
	return parentSessionID()
}

func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func homeDir(options Options) (string, error) {
	if options.Home != nil {
		return options.Home()
	}
	return os.UserHomeDir()
}
func workDir(options Options) (string, error) {
	if options.WorkDir != nil {
		return options.WorkDir()
	}
	return os.Getwd()
}
func cacheDir(options Options) (string, error) {
	if options.Cache != nil {
		return options.Cache()
	}
	return os.UserCacheDir()
}
func stat(options Options, name string) (fs.FileInfo, error) {
	if options.Stat != nil {
		return options.Stat(name)
	}
	return os.Stat(name)
}
func mkdirAll(options Options, path string, mode fs.FileMode) error {
	if options.MkdirAll != nil {
		return options.MkdirAll(path, mode)
	}
	return os.MkdirAll(path, mode)
}
func openFile(options Options, name string, flag int, mode fs.FileMode) (io.Closer, error) {
	if options.OpenFile != nil {
		return options.OpenFile(name, flag, mode)
	}
	return os.OpenFile(name, flag, mode)
}
