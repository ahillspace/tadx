//go:build linux || darwin

package managedpolicy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func secureRead(path string) ([]byte, []ProtectionCheck, error) {
	checks := []ProtectionCheck{}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, checks, errors.New("managed policy requires a clean absolute path")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, checks, errors.New("cannot open managed policy root")
	}
	opened := []int{fd}
	defer func() {
		for _, current := range opened {
			unix.Close(current)
		}
	}()
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	current := "/"
	var protectionErr error
	for index := -1; index < len(parts); index++ {
		file := index == len(parts)-1
		if index >= 0 {
			current = filepath.Join(current, parts[index])
			flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
			if !file {
				flags |= unix.O_DIRECTORY
			}
			next, err := unix.Openat(fd, parts[index], flags, 0)
			if errors.Is(err, unix.ENOENT) {
				return nil, checks, os.ErrNotExist
			}
			if err != nil {
				checks = append(checks, ProtectionCheck{Path: current, Kind: "open", Reason: "cannot open policy path without following links"})
				return nil, checks, errors.New("cannot open managed policy path without following links")
			}
			fd = next
			opened = append(opened, fd)
		}
		var stat unix.Stat_t
		if err := unix.Fstat(fd, &stat); err != nil {
			return nil, checks, errors.New("cannot inspect managed policy path")
		}
		checkErr := checkUnixStat(&stat, file)
		if checkErr == nil {
			checkErr = checkExtendedACL(fd)
		}
		check := ProtectionCheck{Path: current, Kind: "owner-mode-acl-and-links", Passed: checkErr == nil}
		if checkErr != nil {
			check.Reason = checkErr.Error()
		}
		checks = append(checks, check)
		if protectionErr == nil {
			protectionErr = checkErr
		}
		if file {
			if protectionErr != nil {
				return nil, checks, fmt.Errorf("insecure managed policy path: %s", protectionErr)
			}
			opened = opened[:len(opened)-1]
			f := os.NewFile(uintptr(fd), current)
			defer f.Close()
			data, err := readBounded(f)
			if err != nil {
				return nil, checks, err
			}
			var after unix.Stat_t
			if err := unix.Fstat(fd, &after); err != nil || stat.Dev != after.Dev || stat.Ino != after.Ino || stat.Mode != after.Mode || stat.Uid != after.Uid || stat.Gid != after.Gid || stat.Size != after.Size || stat.Mtim != after.Mtim || stat.Ctim != after.Ctim {
				return nil, checks, errors.New("managed policy changed during reading")
			}
			return data, checks, nil
		}
	}
	return nil, checks, errors.New("invalid managed policy path")
}

func checkUnixStat(stat *unix.Stat_t, file bool) error {
	if stat.Uid != 0 {
		return errors.New("owner must be root")
	}
	if stat.Mode&0022 != 0 {
		return errors.New("group and other principals must not have write permission")
	}
	if file && (stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1) {
		return errors.New("policy must be a regular file with one link")
	}
	if !file && stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.New("policy ancestor must be a directory")
	}
	return nil
}
