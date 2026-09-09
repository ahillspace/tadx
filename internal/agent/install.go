// Package agent installs embedded agent skills into bounded global directories.
package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed skills
var bundles embed.FS

// Skill describes one package without exposing machine-specific paths.
type Skill struct {
	Name, Status, Path, SHA256, Backup string
	Files                              int
}

// Result describes both packages and any recoverable backups.
type Result struct {
	Status   string
	Skills   []Skill
	Warnings []string
}

// Installer resolves the user home directory at runtime.
type Installer struct {
	Home func() (string, error)
	// removeAll permits deterministic testing of cleanup failures after commit.
	removeAll func(*os.Root, string) error
	// commitReceipt permits deterministic failure injection at the atomic commit.
	commitReceipt func(*os.Root, string, string) error
}

type packagePlan struct {
	skill     Skill
	files     map[string][]byte
	before    string
	stage     string
	backup    string
	committed bool
	remove    bool
	hidden    bool
}

// Uninstall removes bundled packages from one supported agent target.
// Divergent packages require force and move to recoverable backups.
func (in Installer) Uninstall(ctx context.Context, target string, preview, force bool) (Result, error) {
	base, ok := map[string]string{"claude": ".claude/skills", "codex": ".codex/skills", "cursor": ".cursor/skills"}[target]
	if !ok {
		return Result{}, errors.New("unsupported agent target")
	}
	if in.Home == nil {
		return Result{}, errors.New("user home resolution is not configured")
	}
	home, err := in.Home()
	if err != nil || strings.TrimSpace(home) == "" || !filepath.IsAbs(home) {
		return Result{}, errors.New("cannot resolve an absolute user home directory")
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return Result{}, errors.New("cannot open the user home directory")
	}
	defer root.Close()
	if err := checkParents(root, base); err != nil {
		return Result{}, err
	}
	receipt, err := readReceipt(root, base)
	if err != nil {
		return Result{}, err
	}
	var plans []*packagePlan
	result := Result{Status: "unchanged"}
	for _, name := range []string{"tadx", "tadx-pulse"} {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		files, err := readBundle(name)
		if err != nil {
			return Result{}, err
		}
		destination := path.Join(base, name)
		before, err := fingerprint(root, destination)
		if err != nil {
			return Result{}, fmt.Errorf("cannot inspect %s: %s", name, err)
		}
		status := "remove"
		if before == "" {
			status = "absent"
		} else if before != bundleFingerprint(files) && before != receipt.Packages[name] {
			status = "divergent"
			if !force {
				result.Warnings = append(result.Warnings, name+" differs from the bundled skill; --force is required and retains a backup")
			}
		}
		plans = append(plans, &packagePlan{skill: Skill{Name: name, Status: status, Path: destination, SHA256: before}, before: before})
	}
	if target == "codex" {
		legacy, err := legacyPlans(ctx, root)
		if err != nil {
			return Result{}, err
		}
		plans = append(plans, legacy...)
		for _, plan := range legacy {
			if plan.skill.Status == "divergent" && !force {
				result.Warnings = append(result.Warnings, "Legacy "+plan.skill.Name+" differs; --force is required and retains a backup")
			}
		}
	}
	if preview {
		result.Status = "preview"
		for _, plan := range plans {
			if !plan.hidden {
				result.Skills = append(result.Skills, plan.skill)
			}
		}
		return result, nil
	}
	for _, plan := range plans {
		if plan.skill.Status == "divergent" && !force {
			return Result{}, fmt.Errorf("%s differs from the bundled skill; --force is required to uninstall it with a recoverable backup", plan.skill.Name)
		}
	}
	changed := false
	for _, plan := range plans {
		changed = changed || plan.before != ""
	}
	if !changed {
		for _, plan := range plans {
			if !plan.hidden {
				result.Skills = append(result.Skills, plan.skill)
			}
		}
		return result, nil
	}
	unlock, err := lockPackages(root, target, base)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	if err := receiptUnchanged(root, base, receipt); err != nil {
		return Result{}, err
	}
	for _, plan := range plans {
		current, err := fingerprint(root, plan.skill.Path)
		if err != nil || current != plan.before {
			return Result{}, errors.New("installed skills changed during preparation; retry after reviewing them")
		}
	}
	rollback := func(cause error) (Result, error) {
		for index := len(plans) - 1; index >= 0; index-- {
			plan := plans[index]
			if plan.stage != "" {
				if err := root.Rename(plan.stage, plan.skill.Path); err != nil {
					return Result{}, errors.New("uninstall failed and rollback is incomplete; inspect target packages")
				}
			}
		}
		return Result{}, cause
	}
	for _, plan := range plans {
		if plan.before == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return rollback(err)
		}
		stage, err := removalDestination(root, plan)
		if err != nil {
			return rollback(err)
		}
		if err := root.Rename(plan.skill.Path, stage); err != nil {
			return rollback(errors.New("cannot stage the installed skill for removal"))
		}
		plan.stage = stage
		current, err := fingerprint(root, stage)
		if err != nil || current != plan.before {
			return rollback(errors.New("installed skill changed before removal; previous package restored"))
		}
	}
	if len(receipt.raw) > 0 {
		if err := in.writeReceipt(root, base, map[string]string{}); err != nil {
			return rollback(err)
		}
	}
	for _, plan := range plans {
		if plan.before == "" {
			if !plan.hidden {
				result.Skills = append(result.Skills, plan.skill)
			}
			continue
		}
		if plan.skill.Status == "divergent" {
			plan.skill.Status = "backed-up"
			plan.skill.Backup = plan.stage
			plan.stage = ""
			result.Warnings = append(result.Warnings, "Divergent "+plan.skill.Name+" package retained as a backup; use --full for its home-relative path")
		} else {
			in.cleanupRemoval(root, plan, &result)
			plan.stage = ""
			plan.skill.Status = "removed"
		}
		result.Skills = append(result.Skills, plan.skill)
	}
	result.Status = "uninstalled"
	return result, nil
}

// Install stages complete packages before replacing destinations.
// A force replacement retains the previous directory as a recoverable backup.
func (in Installer) Install(ctx context.Context, target string, preview, force bool) (Result, error) {
	base, ok := map[string]string{"claude": ".claude/skills", "codex": ".codex/skills", "cursor": ".cursor/skills"}[target]
	if !ok {
		return Result{}, errors.New("unsupported agent target")
	}
	if in.Home == nil {
		return Result{}, errors.New("user home resolution is not configured")
	}
	home, err := in.Home()
	if err != nil || strings.TrimSpace(home) == "" || !filepath.IsAbs(home) {
		return Result{}, errors.New("cannot resolve an absolute user home directory")
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return Result{}, errors.New("cannot open the user home directory")
	}
	defer root.Close()
	if err := checkParents(root, base); err != nil {
		return Result{}, err
	}
	receipt, err := readReceipt(root, base)
	if err != nil {
		return Result{}, err
	}
	var plans []*packagePlan
	for _, name := range []string{"tadx", "tadx-pulse"} {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		files, err := readBundle(name)
		if err != nil {
			return Result{}, err
		}
		destination := path.Join(base, name)
		before, err := fingerprint(root, destination)
		if err != nil {
			return Result{}, fmt.Errorf("cannot inspect %s: %s", name, err)
		}
		digest := bundleFingerprint(files)
		status := "install"
		if before == digest {
			status = "unchanged"
		} else if before != "" {
			status = "replace"
			if before == receipt.Packages[name] {
				status = "upgrade"
			}
		}
		plans = append(plans, &packagePlan{skill: Skill{Name: name, Status: status, Path: destination, SHA256: digest, Files: len(files)}, files: files, before: before})
	}
	if target == "codex" {
		legacy, err := legacyPlans(ctx, root)
		if err != nil {
			return Result{}, err
		}
		plans = append(plans, legacy...)
	}
	result := Result{Status: "unchanged"}
	for _, plan := range plans {
		if (plan.skill.Status == "replace" || plan.skill.Status == "divergent") && !force {
			if !preview {
				return Result{}, fmt.Errorf("%s differs from the bundled skill; --force is required to replace it with a recoverable backup", plan.skill.Name)
			}
			result.Warnings = append(result.Warnings, plan.skill.Name+" differs; installation requires --force and retains a backup")
		}
	}
	if preview {
		result.Status = "preview"
		for _, plan := range plans {
			if !plan.hidden {
				result.Skills = append(result.Skills, plan.skill)
			}
		}
		return result, nil
	}
	changed := false
	for _, plan := range plans {
		changed = changed || (!plan.remove && plan.skill.Status != "unchanged") || (plan.remove && plan.before != "")
	}
	if err := root.MkdirAll(base, 0o755); err != nil {
		return Result{}, errors.New("cannot create the target skill directory")
	}
	if err := checkParents(root, base); err != nil {
		return Result{}, err
	}
	unlock, err := lockPackages(root, target, base)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	if err := receiptUnchanged(root, base, receipt); err != nil {
		return Result{}, err
	}
	defer func() {
		for _, plan := range plans {
			if plan.stage != "" {
				_ = root.RemoveAll(plan.stage)
			}
		}
	}()
	for _, plan := range plans {
		if plan.skill.Status == "unchanged" || plan.remove {
			continue
		}
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		plan.stage = path.Join(base, ".tadx-stage-"+rand.Text())
		if err := root.Mkdir(plan.stage, 0o700); err != nil {
			return Result{}, errors.New("cannot create a skill staging directory")
		}
		for name, data := range plan.files {
			location := path.Join(plan.stage, name)
			if err := root.MkdirAll(path.Dir(location), 0o755); err != nil {
				return Result{}, errors.New("cannot create a staged skill directory")
			}
			if err := root.WriteFile(location, data, 0o644); err != nil {
				return Result{}, errors.New("cannot write a staged skill file")
			}
		}
		if err := root.Chmod(plan.stage, 0o755); err != nil {
			return Result{}, errors.New("cannot set staged skill permissions")
		}
	}
	// Validate every destination before the first commit, including unchanged skills.
	for _, plan := range plans {
		current, err := fingerprint(root, plan.skill.Path)
		if err != nil || current != plan.before {
			return Result{}, errors.New("installed skills changed during preparation; retry after reviewing them")
		}
	}
	rollback := func(cause error) (Result, error) {
		for index := len(plans) - 1; index >= 0; index-- {
			plan := plans[index]
			if plan.committed {
				// Move the new package back into its owned staging location.
				if err := root.Rename(plan.skill.Path, plan.stage); err != nil {
					return Result{}, errors.New("installation failed and rollback is incomplete; inspect target packages and .tadx-skill-backups")
				}
			}
			if plan.backup != "" {
				if err := root.Rename(plan.backup, plan.skill.Path); err != nil {
					return Result{}, errors.New("installation failed and rollback is incomplete; inspect target packages and .tadx-skill-backups")
				}
			}
		}
		return Result{}, cause
	}
	for _, plan := range plans {
		if plan.skill.Status == "unchanged" {
			continue
		}
		if plan.remove {
			if plan.before == "" {
				continue
			}
			if err := ctx.Err(); err != nil {
				return rollback(err)
			}
			backup, err := removalDestination(root, plan)
			if err != nil {
				return rollback(err)
			}
			if err := root.Rename(plan.skill.Path, backup); err != nil {
				return rollback(errors.New("cannot stage legacy skill removal"))
			}
			plan.backup = backup
			current, err := fingerprint(root, backup)
			if err != nil || current != plan.before {
				return rollback(errors.New("legacy skill changed before removal; previous package restored"))
			}
			continue
		}
		if err := ctx.Err(); err != nil {
			return rollback(err)
		}
		if plan.before != "" {
			backupBase := path.Join(path.Dir(base), ".tadx-skill-backups")
			if err := checkParents(root, backupBase); err != nil {
				return rollback(err)
			}
			if err := root.MkdirAll(backupBase, 0o700); err != nil {
				return rollback(errors.New("cannot create the skill backup directory"))
			}
			backup := path.Join(backupBase, plan.skill.Name+"-"+rand.Text())
			if err := root.Rename(plan.skill.Path, backup); err != nil {
				return rollback(errors.New("cannot preserve the existing skill package"))
			}
			plan.backup = backup
			current, err := fingerprint(root, backup)
			if err != nil || current != plan.before {
				return rollback(errors.New("installed skill changed before replacement; previous package restored"))
			}
		}
		if err := root.Rename(plan.stage, plan.skill.Path); err != nil {
			return rollback(errors.New("cannot commit the staged skill package"))
		}
		plan.committed = true
		plan.skill.Status = "installed"
		if plan.backup != "" {
			plan.skill.Status = "replaced"
			plan.skill.Backup = plan.backup
			result.Warnings = append(result.Warnings, "Previous "+plan.skill.Name+" package retained as a backup; use --full for its home-relative path")
		}
	}
	packages := map[string]string{}
	for _, plan := range plans {
		if !plan.remove {
			packages[plan.skill.Name] = plan.skill.SHA256
		}
	}
	if err := in.writeReceipt(root, base, packages); err != nil {
		return rollback(err)
	}
	if changed {
		result.Status = "installed"
	}
	for _, plan := range plans {
		if plan.remove && plan.before != "" {
			if plan.skill.Status == "divergent" {
				plan.skill.Status = "backed-up"
				plan.skill.Backup = plan.backup
				result.Warnings = append(result.Warnings, "Divergent legacy "+plan.skill.Name+" retained as a backup; use --full for its home-relative path")
			} else {
				plan.stage = plan.backup
				in.cleanupRemoval(root, plan, &result)
				plan.stage = ""
				plan.skill.Status = "removed"
			}
		}
		if !plan.hidden {
			result.Skills = append(result.Skills, plan.skill)
		}
	}
	return result, nil
}

func checkParents(root *os.Root, base string) error {
	current := ""
	for _, component := range strings.Split(base, "/") {
		current = path.Join(current, component)
		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("target skill directory must contain only real directories inside the user home")
		}
	}
	return nil
}

func readBundle(name string) (map[string][]byte, error) {
	files := make(map[string][]byte)
	prefix := "skills/" + name
	err := fs.WalkDir(bundles, prefix, func(location string, entry fs.DirEntry, err error) error {
		if err != nil {
			return errors.New("cannot read bundled skill")
		}
		if entry.IsDir() {
			return nil
		}
		relative := strings.TrimPrefix(location, prefix+"/")
		if !fs.ValidPath(relative) || strings.Contains(relative, "\\") || strings.Contains(relative, ":") || !entry.Type().IsRegular() {
			return errors.New("bundled skill contains an unsafe path")
		}
		data, err := bundles.ReadFile(location)
		if err != nil {
			return errors.New("cannot read bundled skill file")
		}
		files[relative] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files["SKILL.md"]) == 0 {
		return nil, errors.New("bundled skill is missing SKILL.md")
	}
	return files, nil
}

func bundleFingerprint(files map[string][]byte) string {
	var records []string
	for name, data := range files {
		records = append(records, fmt.Sprintf("%s\x00%x", name, sha256.Sum256(data)))
	}
	// Sort the records independently of embedded or filesystem iteration order.
	sort.Strings(records)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(records, "\n"))))
}

func fingerprint(root *os.Root, directory string) (string, error) {
	info, err := root.Lstat(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("skill destination is not a readable real directory")
	}
	source := root.FS()
	files := make(map[string][]byte)
	var total int64
	entries := 0
	err = fs.WalkDir(source, directory, func(location string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("cannot inspect an installed skill entry")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("installed skills must not contain symbolic links")
		}
		entries++
		if entries > 2048 {
			return errors.New("installed skill exceeds inspection bounds")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("installed skills must contain regular files")
		}
		total += info.Size()
		if len(files) >= 2048 || total > 16<<20 {
			return errors.New("installed skill exceeds inspection bounds")
		}
		file, err := root.Open(location)
		if err != nil {
			return errors.New("cannot open an installed skill file")
		}
		data, readErr := io.ReadAll(io.LimitReader(file, info.Size()+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return errors.New("cannot read an installed skill file")
		}
		if int64(len(data)) != info.Size() {
			return errors.New("installed skill changed during inspection")
		}
		files[strings.TrimPrefix(location, directory+"/")] = data
		return nil
	})
	if err != nil {
		return "", err
	}
	return bundleFingerprint(files), nil
}
