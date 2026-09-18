//go:build linux || darwin

package managedpolicy

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

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
	dir := t.TempDir()
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
