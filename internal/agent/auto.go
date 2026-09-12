package agent

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/ahillspace/tadx/internal/agenttarget"
)

// installAuto refreshes only supported agent homes already present, or installs
// portable Guidance when no client can be detected. It never writes client config.
func (in Installer) installAuto(ctx context.Context, preview, force bool) (Result, error) {
	if in.Home == nil {
		return Result{}, errors.New("user home resolution is not configured")
	}
	home, err := in.Home()
	if err != nil || !filepath.IsAbs(home) {
		return Result{}, errors.New("cannot resolve an absolute user home directory")
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return Result{}, errors.New("cannot open the user home directory")
	}
	defer root.Close()
	var targets []string
	for _, target := range agenttarget.SupportedTargets() {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		base, _ := agenttarget.TargetPath(target)
		config := path.Dir(base)
		if target == "pi" {
			config = ".pi"
		}
		_, err := root.Lstat(config)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return Result{}, packageFilesystemError("inspect agent configuration", target, err)
		}
		// Reject links even when they point back inside home. Never silently change
		// the detection set in response to an unsafe existing target.
		if err := checkParents(root, config); err != nil {
			return Result{}, fmt.Errorf("%s: %w", target, err)
		}
		targets = append(targets, target)
	}
	result := Result{Status: "unchanged", Targets: targets}
	if len(targets) == 0 {
		targets = []string{"generic"}
		result.Targets = targets
		result.Warnings = []string{"No supported agent configuration detected; portable Guidance uses .agents/skills. Select --target explicitly if your agent does not discover that directory."}
	}
	// Resolve home once for the whole operation, even when a caller supplied a
	// dynamic resolver. Each target retains its own atomic package transaction.
	in.Home = func() (string, error) { return home, nil }
	var failures []error
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		installed, err := in.Install(ctx, target, preview, force)
		for _, skill := range installed.Skills {
			skill.Target = target
			result.Skills = append(result.Skills, skill)
		}
		result.Warnings = append(result.Warnings, installed.Warnings...)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", target, err))
			result.Skills = append(result.Skills, Skill{Target: target, Name: "tadx", Status: "failed"})
		} else if installed.Status == "installed" {
			result.Status = "installed"
		}
	}
	if len(failures) > 0 {
		result.Status = "partial"
		return result, errors.Join(failures...)
	}
	if preview {
		result.Status = "preview"
	}
	return result, nil
}
