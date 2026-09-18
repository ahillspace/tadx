//go:build linux || darwin

package managedpolicy

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/unix"
)

// Elevated native CI can exercise the complete root-owned path without touching
// the deployed TADX location. Only this disposable directory is ever modified.
func TestPrivilegedUnixProtectedPolicyFixture(t *testing.T) {
	if os.Geteuid() != 0 {
		if os.Getenv("TADX_TEST_REQUIRE_PRIVILEGED_POLICY") == "1" {
			t.Fatal("dedicated native policy job requires root")
		}
		t.Skip("requires an elevated native runner for a root-owned fixture")
	}
	base := "/root"
	if runtime.GOOS == "darwin" {
		base = "/private/var/root"
	}
	dir, err := os.MkdirTemp(base, "tadx-policy-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	path := filepath.Join(dir, "candidate.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"allowed_capabilities":["read"],"remote_mutations":false}`), 0644); err != nil {
		t.Fatal(err)
	}
	p := loadPath(path, testCatalog())
	if status := p.Status(); status.State != StateActive || !status.Protected || !status.CandidateValid {
		t.Fatalf("protected policy rejected: %#v", status)
	}
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if p := loadPath(path, testCatalog()); p.Status().State != StateError {
		t.Fatalf("writable file accepted: %#v", p.Status())
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(dir, "other-link")); err != nil {
		t.Fatal(err)
	}
	if p := loadPath(path, testCatalog()); p.Status().State != StateError {
		t.Fatalf("hardlink accepted: %#v", p.Status())
	}
	fifo := filepath.Join(dir, "fifo")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if p := loadPath(fifo, testCatalog()); p.Status().State != StateError {
		t.Fatalf("FIFO accepted: %#v", p.Status())
	}
}

func TestUnixAccessDeniedIsNotUnmanaged(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses ordinary file access denial")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	if p := loadPath(filepath.Join(dir, "candidate.json"), testCatalog()); p.Status().State != StateError {
		t.Fatalf("access denial treated as absence: %#v", p.Status())
	}
}
