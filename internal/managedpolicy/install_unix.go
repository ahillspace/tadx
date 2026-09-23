//go:build linux || darwin

package managedpolicy

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/capability"
	"golang.org/x/sys/unix"
)

func Install(ctx context.Context, options InstallOptions, definitions []capability.Definition) (InstallResult, error) {
	return runUnixInstall(ctx, options, definitions, os.Geteuid() == 0, elevateUnixInstall, unixLocatorPath(), defaultUnixPolicyPath())
}

func runUnixInstall(ctx context.Context, options InstallOptions, definitions []capability.Definition, root bool, elevate func(context.Context, InstallResult) (InstallResult, error), locator, defaultPath string) (InstallResult, error) {
	out, data, err := prepareUnixInstall(options, definitions, defaultPath)
	if err != nil {
		return out, err
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if !root {
		return elevate(ctx, out)
	}
	return installWith(ctx, out, data, &unixInstaller{locator: locator, defaultPath: defaultPath, definitions: definitions, directory: -1})
}

func prepareUnixInstall(options InstallOptions, definitions []capability.Definition, defaultPath string) (InstallResult, []byte, error) {
	out := InstallResult{Template: options.Template, Phase: "validation"}
	if out.Template == "" || out.Template == "admin" {
		out.Template = "superuser"
	}
	doc, err := Template(out.Template, definitions)
	if err != nil {
		return out, nil, err
	}
	dir := options.Directory
	if dir == "" {
		dir = filepath.Dir(defaultPath)
	}
	if err := validateUnixDirectory(dir); err != nil {
		return out, nil, err
	}
	out.Path = filepath.Join(dir, "managed-policy.json")
	data, err := json.MarshalIndent(doc, "", "  ")
	return out, append(data, '\n'), err
}

func validateUnixDirectory(dir string) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || strings.ContainsRune(dir, 0) || dir == "/" {
		return errors.New("policy directory must be a clean absolute path to a dedicated directory")
	}
	if slices.Contains([]string{"/etc", "/private", "/private/etc", "/private/var", "/private/var/root", "/root", "/home", "/Users", "/Library", "/Library/Application Support", "/usr", "/usr/local", "/opt", "/var", "/tmp", "/private/tmp"}, dir) || filepath.Dir(dir) == "/home" || filepath.Dir(dir) == "/Users" {
		return errors.New("choose a dedicated policy directory, not a shared system directory or user home")
	}
	return nil
}

func sameInstallPath(a, b string) bool { return a != "" && a == b }

type unixInstaller struct {
	locator, defaultPath string
	definitions          []capability.Definition
	directory            int
	writeSyncErr         error
}

func (u *unixInstaller) current() (string, error) {
	path, _, err := readUnixLocation(u.locator, u.defaultPath)
	return path, err
}

func (u *unixInstaller) lock(ctx context.Context) (func(), error) {
	dir, _, err := openUnixDirectory(filepath.Dir(u.locator), false)
	if err != nil {
		return nil, err
	}
	defer unix.Close(dir)
	fd, err := unix.Openat(dir, ".tadx-policy-install.lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	if err := inspectUnixFD(fd, true); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("unsafe policy installation lock: %w", err)
	}
	for {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { _ = unix.Flock(fd, unix.LOCK_UN); _ = unix.Close(fd) }, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			unix.Close(fd)
			return nil, err
		}
		select {
		case <-ctx.Done():
			unix.Close(fd)
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (u *unixInstaller) prepare(dir string) (func(), bool, error) {
	fd, created, err := openUnixDirectory(dir, true)
	if err != nil {
		return nil, created, err
	}
	u.directory = fd
	return func() { _ = unix.Close(fd) }, created, onlyUnixPolicyContents(fd)
}

func (u *unixInstaller) write(_ string, data []byte) error {
	if err := onlyUnixPolicyContents(u.directory); err != nil {
		return err
	}
	written, err := replaceUnixFile(u.directory, "managed-policy.json", data)
	if written {
		// A durability failure after rename is still a confirmed policy write.
		u.writeSyncErr = err
		return nil
	}
	return err
}

func (u *unixInstaller) verify(path string) error {
	if u.writeSyncErr != nil {
		return u.writeSyncErr
	}
	if status := loadPath(path, u.definitions).Status(); status.State != StateActive {
		return fmt.Errorf("installed policy verification failed: %s", status.Reason)
	}
	return nil
}

func (u *unixInstaller) publish(dir string) (bool, error) {
	fd, _, err := openUnixDirectory(filepath.Dir(u.locator), false)
	if err != nil {
		return false, err
	}
	defer unix.Close(fd)
	data, err := json.Marshal(dir)
	if err != nil {
		return false, err
	}
	published, err := replaceUnixFile(fd, filepath.Base(u.locator), append(data, '\n'))
	if err != nil {
		return published, err
	}
	path, _, err := readUnixLocation(u.locator, u.defaultPath)
	if err == nil && path != filepath.Join(dir, "managed-policy.json") {
		err = errors.New("policy locator verification returned a different directory")
	}
	return true, err
}

// Every component is resolved relative to an inspected directory descriptor.
// Existing ancestors and the leaf are never repaired or given new permissions.
func openUnixDirectory(path string, createLeaf bool) (int, bool, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, 0) {
		return -1, false, errors.New("directory requires a clean absolute path")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, false, err
	}
	created := false
	if err := inspectUnixFD(fd, false); err != nil {
		unix.Close(fd)
		return -1, false, err
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if path == "/" {
		parts = nil
	}
	for i, part := range parts {
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(err, unix.ENOENT) && createLeaf && i == len(parts)-1 {
			err = unix.Mkdirat(fd, part, 0755)
			if err == nil {
				created = true
				next, err = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
				if err == nil {
					err = unix.Fchmod(next, 0755)
					if err != nil {
						unix.Close(next)
					}
				}
			}
		}
		unix.Close(fd)
		if err != nil {
			return -1, created, fmt.Errorf("cannot open protected policy directory (parent must exist; symlinks are forbidden): %w", err)
		}
		fd = next
		if err := inspectUnixFD(fd, false); err != nil {
			unix.Close(fd)
			return -1, created, fmt.Errorf("insecure policy directory: %w", err)
		}
	}
	return fd, created, nil
}

func inspectUnixFD(fd int, file bool) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if err := checkUnixStat(&stat, file); err != nil {
		return err
	}
	return checkExtendedACL(fd)
}

func inspectUnixFileAt(dir int, name string) error {
	fd, err := unix.Openat(dir, name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return inspectUnixFD(fd, true)
}

func onlyUnixPolicyContents(fd int) error {
	copyFD, err := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(copyFD), "policy directory")
	defer file.Close()
	entries, err := file.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry != "managed-policy.json" {
			return errors.New("policy installation requires a dedicated directory containing no unrelated files or directories")
		}
		if err := inspectUnixFileAt(fd, entry); err != nil {
			return fmt.Errorf("unsafe existing policy: %w", err)
		}
	}
	return nil
}

// Report rename separately from durability so the receipt never loses a write.
func replaceUnixFile(dir int, name string, data []byte) (bool, error) {
	if err := inspectUnixFileAt(dir, name); err != nil {
		return false, err
	}
	temp := ".tadx-policy-" + rand.Text() + ".tmp"
	fd, err := unix.Openat(dir, temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return false, err
	}
	defer unix.Unlinkat(dir, temp, 0)
	file := os.NewFile(uintptr(fd), temp)
	defer file.Close()
	if err := unix.Fchmod(fd, 0644); err != nil {
		return false, err
	}
	if err := inspectUnixFD(fd, true); err != nil {
		return false, err
	}
	if _, err := file.Write(data); err != nil {
		return false, err
	}
	if err := file.Sync(); err != nil {
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	if err := unix.Renameat(dir, temp, dir, name); err != nil {
		return false, err
	}
	return true, unix.Fsync(dir)
}
