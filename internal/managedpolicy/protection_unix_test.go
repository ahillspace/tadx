//go:build linux || darwin

package managedpolicy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestUnixExecutableInspection(t *testing.T) {
	// This fixture has execute permission but no read permission, like sudo
	// on macOS. An ordinary user's ownership should be rejected after opening.
	dir := t.TempDir()
	path := filepath.Join(dir, "executable")
	if err := os.WriteFile(path, []byte("fixture"), 0111); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	opened, err := inspectUnixExecutableAt(fd, "executable")
	if os.Geteuid() == 0 {
		if !opened || err != nil {
			t.Fatalf("execute-only inspection as root: opened=%t error=%v", opened, err)
		}
	} else if !opened || err == nil || !strings.Contains(err.Error(), "owner must be root") {
		t.Fatalf("execute-only inspection as ordinary user: opened=%t error=%v", opened, err)
	}
	if err := os.Symlink(path, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if opened, err := inspectUnixExecutableAt(fd, "link"); err == nil {
		t.Fatalf("symlink accepted: opened=%t", opened)
	}
	if opened, err := inspectUnixExecutableAt(fd, "missing"); opened || !os.IsNotExist(err) {
		t.Fatalf("missing executable: opened=%t error=%v", opened, err)
	}
}

func TestUnixModeContract(t *testing.T) {
	for _, tc := range []struct {
		name        string
		uid         uint32
		mode        uint32
		links       uint64
		file, valid bool
	}{
		{"protected-file", 0, unix.S_IFREG | 0644, 1, true, true},
		{"protected-directory", 0, unix.S_IFDIR | 0755, 1, false, true},
		{"user-owner", 1000, unix.S_IFREG | 0644, 1, true, false},
		{"group-write", 0, unix.S_IFREG | 0664, 1, true, false},
		{"other-write", 0, unix.S_IFREG | 0646, 1, true, false},
		{"directory-write", 0, unix.S_IFDIR | 0777, 1, false, false},
		{"hardlink", 0, unix.S_IFREG | 0644, 2, true, false},
		{"symlink", 0, unix.S_IFLNK | 0644, 1, true, false},
		{"fifo", 0, unix.S_IFIFO | 0644, 1, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stat unix.Stat_t
			stat.Uid = tc.uid
			// Field widths differ between Darwin and Linux.
			setTestStat(&stat, tc.mode, tc.links)
			if err := checkUnixStat(&stat, tc.file); (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}

func TestUnixTemporaryPolicyProtection(t *testing.T) {
	// macOS temporary paths can traverse /var, which is itself a symlink.
	// Resolve the fixture root; the loader must still reject linked policy paths.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "managed-policy.json")
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatal(err)
	}
	if p := loadPath(path, testCatalog()); p.Status().State != StateUnmanaged {
		t.Fatalf("missing policy: %#v", p.Status())
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"allowed_capabilities":[],"remote_mutations":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if p := loadPath(path, testCatalog()); p.Status().State != StateError {
		t.Fatalf("insecure ancestor accepted: %#v", p.Status())
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "absent"), link); err != nil {
		t.Fatal(err)
	}
	if p := loadPath(link, testCatalog()); p.Status().State != StateError {
		t.Fatalf("dangling symlink accepted: %#v", p.Status())
	}
}
