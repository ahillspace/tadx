// Package update reuses the supported bootstrap installers for native updates.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	action "github.com/ahillspace/tadx/actions/update"
	"github.com/ahillspace/tadx/internal/agenttarget"
	"github.com/ahillspace/tadx/internal/version"
	"github.com/ahillspace/tadx/scripts"
)

var releaseVersion = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

// Runtime withholds Tableau PAT variables from every updater child process.
// Conventional TADX_*_PAT_NAME and TADX_*_PAT_SECRET names are always withheld.
type Runtime struct {
	// CredentialVariables returns the configured PAT variable references.
	CredentialVariables func() []string
}

func (Runtime) Current() string { return version.Current() }
func (Runtime) InstallationTarget() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return filepath.ToSlash(executable), err
	}
	return filepath.ToSlash(resolved), nil
}
func (Runtime) ValidateTargets(targets []string) error {
	for _, target := range targets {
		if target != "auto" && !agenttarget.IsSupported(target) {
			return errors.New("invalid agent target; use auto or a supported agent target")
		}
	}
	return nil
}
func (r Runtime) Latest(ctx context.Context) (action.Release, error) {
	run := r.runner()
	// gh supplies authenticated access while the repository remains private.
	if path, err := exec.LookPath("gh"); err == nil {
		body, err := run(ctx, 30*time.Second, path, "release", "view", "--repo", "ahillspace/tadx", "--json", "tagName,url")
		if err == nil {
			var r struct {
				Tag string `json:"tagName"`
				URL string `json:"url"`
			}
			if json.Unmarshal(body, &r) == nil && releaseVersion.MatchString(r.Tag) && strings.HasPrefix(r.URL, "https://github.com/ahillspace/tadx/releases/tag/") {
				return action.Release{Version: strings.TrimPrefix(r.Tag, "v"), URL: r.URL}, nil
			}
			return action.Release{}, errors.New("GitHub returned an invalid release identity")
		}
	}
	release, err := (version.Checker{}).Latest(ctx)
	if err != nil {
		return action.Release{}, err
	}
	if !releaseVersion.MatchString(release.Version) {
		return action.Release{}, errors.New("GitHub returned an invalid release version")
	}
	return action.Release{Version: release.Version, URL: release.URL}, nil
}
func (r Runtime) Install(ctx context.Context, release action.Release, targets []string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	expected := "tadx"
	if runtime.GOOS == "windows" {
		expected += ".exe"
	}
	if !strings.EqualFold(filepath.Base(executable), expected) {
		return errors.New("run tadx update from the installed tadx executable")
	}
	program, err := installerProgram()
	if err != nil {
		return err
	}
	return install(ctx, runtime.GOOS, program, filepath.Dir(executable), release, targets, r.runner())
}

type runner func(context.Context, time.Duration, string, ...string) ([]byte, error)

// runner binds the child environment once per operation.
func (r Runtime) runner() runner {
	var configured []string
	if r.CredentialVariables != nil {
		configured = r.CredentialVariables()
	}
	environment := childEnvironment(os.Environ(), configured)
	return func(ctx context.Context, timeout time.Duration, program string, args ...string) ([]byte, error) {
		return runProcess(ctx, timeout, environment, program, args...)
	}
}

// childEnvironment drops PAT variables, which neither gh nor the installers read.
// Names compare case-insensitively so Windows spellings cannot bypass the filter.
func childEnvironment(environment, configured []string) []string {
	withheld := make(map[string]bool, len(configured))
	for _, name := range configured {
		if name != "" {
			withheld[strings.ToUpper(name)] = true
		}
	}
	kept := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(name)
		conventional := strings.HasPrefix(upper, "TADX_") && (strings.HasSuffix(upper, "_PAT_NAME") || strings.HasSuffix(upper, "_PAT_SECRET"))
		if !conventional && !withheld[upper] {
			kept = append(kept, entry)
		}
	}
	return kept
}

func install(ctx context.Context, platform, program, directory string, release action.Release, targets []string, execute runner) error {
	if len(targets) == 0 {
		targets = []string{"auto"}
	}
	temp, err := os.MkdirTemp("", "tadx-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	script := scripts.UnixInstaller
	name := "install.sh"
	if platform == "windows" {
		script = scripts.WindowsInstaller
		name = "install.ps1"
	}
	scriptPath := filepath.Join(temp, name)
	if err = os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		return err
	}
	args := []string{scriptPath, "--version", release.Version, "--install-dir", directory, "--no-modify-path", "--no-completion"}
	if platform == "windows" {
		args = []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath, "-Version", release.Version, "-InstallDir", directory, "-NoModifyPath", "-NoCompletion", "-Target", strings.Join(targets, ",")}
	} else {
		for _, target := range targets {
			args = append(args, "--target", target)
		}
	}
	output, err := execute(ctx, 5*time.Minute, program, args...)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("installer and its subprocesses were stopped; installation state may be partial, and rollback is not confirmed. Inspect the installed version and retained .tadx.backup files before retrying: %w", err)
		}
		return fmt.Errorf("installer failed; binary rollback is attempted on Guidance failure: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// Bounded capture never puts an unbounded installer transcript in CLI errors.
type capture struct {
	body  []byte
	limit int
}

func (c *capture) Write(p []byte) (int, error) {
	n := len(p)
	remaining := c.limit - len(c.body)
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		c.body = append(c.body, p...)
	}
	return n, nil
}
func runProcess(ctx context.Context, timeout time.Duration, environment []string, program string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Env = environment
	scope, err := newProcessScope(cmd)
	if err != nil {
		return nil, err
	}
	defer scope.close()
	cmd.WaitDelay = 3 * time.Second
	out := &capture{limit: 16 * 1024}
	cmd.Stdout = out
	cmd.Stderr = out
	err = cmd.Start()
	if err == nil {
		if startErr := scope.started(); startErr != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return out.body, startErr
		}
		err = cmd.Wait()
	}
	if ctx.Err() != nil {
		return out.body, ctx.Err()
	}
	return out.body, err
}
