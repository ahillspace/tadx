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

type Runtime struct{}

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
func (Runtime) Latest(ctx context.Context) (action.Release, error) {
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
func (Runtime) Install(ctx context.Context, release action.Release, targets []string) error {
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
	return install(ctx, runtime.GOOS, filepath.Dir(executable), release, targets, run)
}

type runner func(context.Context, time.Duration, string, ...string) ([]byte, error)

func install(ctx context.Context, platform, directory string, release action.Release, targets []string, execute runner) error {
	if !releaseVersion.MatchString(release.Version) {
		return errors.New("invalid release version")
	}
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
	program := "sh"
	if platform == "windows" {
		script = scripts.WindowsInstaller
		name = "install.ps1"
		program = "powershell.exe"
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
func run(ctx context.Context, timeout time.Duration, program string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...)
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
