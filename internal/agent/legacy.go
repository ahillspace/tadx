package agent

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
)

const legacyCodexBase = ".agents/skills"

// These exact package fingerprints come from the bundled trees at the named
// revisions, using both LF and CRLF checkouts. Names or frontmatter alone never
// establish whether a recoverable backup is needed for an edited package.
var legacyFingerprints = map[string][]string{
	"tadx": {
		"c3dcc032ad0b9e0522d022380fc9d7438a5861801250f4d8af502e34189a3b41", // 4d57544 LF
		"12d7bb6a0629a234beed8cfbf1bbc26a7a7bdbc9183df9ff4113dc8f9c49d736", // 4d57544 CRLF
		"cac857f89378ac2ac147e26b485e911155407e48cbc77b8eb1d0cd1c5dea0836", // 89287ae LF
		"a1ed75e6e66182116a741e5188171b5f17b3b8fddabd562f04bc2497c2bcc8c9", // 89287ae CRLF
		"8fad900131dc335e3ff1e6a02d392ae40a314a3d0c9f8983ba81f42331c0fa37", // 0a0a371 LF
		"bc240280d2b59515af1258fb5f1c8de610ab3b65d12878d184bc46a61bbe50c5", // 0a0a371 CRLF
		"e437aee31186d7eca540d2be742d866e80b1a5af7f1a6ab78edf843b23cce9d3", // 6701e49 LF
		"ed531a523cfa9599f4e728734764eebf68530d903ceb1c7f7584f1bceaeb096c", // 6701e49 CRLF
	},
	"tadx-pulse": {
		"3b5b7b345fbbdafe9d2320f5e54cf135ba343b46060b2f214e7ad65b43a0bfbb", // 4d57544 LF
		"1e2e4e175f69b1ba6e236e3f967a30cf5bcba9fd5881a3211c73d850477cc5e2", // 4d57544 CRLF
		"2dd45958fa809e18a96ff1226af6d351b078d2836135fefd1347e4873935527d", // 89287ae LF
		"d5aa952e974fb75f0cde2b69b63a20ff863941f4586b005166f8986ab2eec960", // 89287ae CRLF
		"4b2a4b42b358eabc78dc47b18f2d35279c7bcb8d57023d2182e231f8080c276d", // 0a0a371, 6701e49 LF
		"b8a868b2e4b31532db15fe9d43edb1d5db791c17b3c7671b5621d6dec0f422ce", // 0a0a371, 6701e49 CRLF
	},
}

func legacyPlans(ctx context.Context, root *os.Root) ([]*packagePlan, error) {
	if err := checkParents(root, legacyCodexBase); err != nil {
		return nil, err
	}
	// A receipt marks the portable target installed by the current installer,
	// not an obsolete Codex package. Keep that independent installation intact.
	receipt, err := readReceipt(root, legacyCodexBase)
	if err != nil {
		return nil, err
	}
	if len(receipt.Packages) != 0 {
		return nil, nil
	}
	var plans []*packagePlan
	for _, name := range []string{"tadx", "tadx-pulse"} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		location := path.Join(legacyCodexBase, name)
		before, err := fingerprint(root, location)
		if err != nil {
			return nil, fmt.Errorf("cannot inspect legacy %s: %s", name, err)
		}
		files, err := readBundle(name)
		if err != nil {
			return nil, err
		}
		status := "divergent"
		if before == "" {
			status = "absent"
		} else if knownLegacy(name, before, bundleFingerprint(files)) {
			status = "remove"
		}
		plans = append(plans, &packagePlan{
			skill:  Skill{Name: name, Status: status, Path: location, SHA256: before},
			before: before, remove: true, hidden: before == "",
		})
	}
	return plans, nil
}

func knownLegacy(name, digest, current string) bool {
	if digest == current {
		return true
	}
	for _, known := range legacyFingerprints[name] {
		if digest == known {
			return true
		}
	}
	return false
}

func legacyOwnershipUnchanged(root *os.Root, plans []*packagePlan) error {
	for _, plan := range plans {
		if plan.remove {
			receipt, err := readReceipt(root, legacyCodexBase)
			if err != nil {
				return err
			}
			if len(receipt.Packages) != 0 {
				return errors.New("portable Guidance was registered during preparation; retry without removing its independent installation")
			}
			break
		}
	}
	return nil
}

// Lock both discoverable roots, including the old installer's lock location.
// Skip absent roots on uninstall so removing legacy-only packages creates no
// canonical skills directory. All instances acquire locks in the same order.
func lockPackages(root *os.Root, target, base string) (func(), error) {
	bases := []string{base}
	if target == "codex" {
		bases = append(bases, legacyCodexBase)
	}
	var locks []string
	unlock := func() {
		for index := len(locks) - 1; index >= 0; index-- {
			_ = root.Remove(locks[index])
		}
	}
	for _, directory := range bases {
		if err := checkParents(root, directory); err != nil {
			unlock()
			return nil, err
		}
		if _, err := root.Lstat(directory); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			unlock()
			return nil, errors.New("cannot inspect the skill lock directory")
		}
		lock := path.Join(directory, ".tadx-install.lock")
		file, err := root.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			unlock()
			return nil, errors.New("cannot acquire the skill installation lock; another install or a stale .tadx-install.lock requires attention")
		}
		locks = append(locks, lock)
		if err := file.Close(); err != nil {
			unlock()
			return nil, errors.New("cannot close the installation lock")
		}
	}
	return unlock, nil
}

// Keep removed packages outside skill discovery even if cleanup fails.
func removalDestination(root *os.Root, plan *packagePlan) (string, error) {
	directory := ".tadx-skill-staging"
	if plan.skill.Status == "divergent" {
		directory = ".tadx-skill-backups"
	}
	base := path.Join(path.Dir(path.Dir(plan.skill.Path)), directory)
	if err := checkParents(root, base); err != nil {
		return "", err
	}
	if err := root.MkdirAll(base, 0o700); err != nil {
		return "", errors.New("cannot create the skill removal directory")
	}
	return path.Join(base, plan.skill.Name+"-"+rand.Text()), nil
}

func (in Installer) cleanupRemoval(root *os.Root, plan *packagePlan, result *Result) {
	remove := in.removeAll
	if remove == nil {
		remove = func(root *os.Root, location string) error { return root.RemoveAll(location) }
	}
	if err := remove(root, plan.stage); err != nil {
		plan.skill.Backup = plan.stage
		result.Warnings = append(result.Warnings, plan.skill.Name+" was removed from skill discovery; staged cleanup failed; use --full for its home-relative recovery path")
	}
}
