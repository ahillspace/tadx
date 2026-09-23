package managedpolicy

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPrivilegedWindowsProtectedPolicyFixture(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		if os.Getenv("TADX_TEST_REQUIRE_PRIVILEGED_POLICY") == "1" {
			t.Fatal("dedicated native policy job requires an elevated administrator token")
		}
		t.Skip("requires an elevated native runner for an administrator-owned fixture")
	}
	// Keep this legacy fixture isolated under the native system directory.
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		t.Fatal(err)
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
	protect := func(path string) {
		t.Helper()
		sd, err := windows.SecurityDescriptorFromString("O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FR;;;BU)")
		if err != nil {
			t.Fatal(err)
		}
		owner, _, err := sd.Owner()
		if err != nil {
			t.Fatal(err)
		}
		dacl, _, err := sd.DACL()
		if err != nil {
			t.Fatal(err)
		}
		if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, dacl, nil); err != nil {
			t.Fatal(err)
		}
	}
	protect(dir)
	path := filepath.Join(dir, "candidate.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"allowed_capabilities":["read"],"remote_mutations":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	protect(path)
	if p := loadPath(path, testCatalog()); p.Status().State != StateActive {
		t.Fatalf("protected policy rejected: %#v", p.Status())
	}
	if err := os.Link(path, filepath.Join(dir, "other-link")); err != nil {
		t.Fatal(err)
	}
	if p := loadPath(path, testCatalog()); p.Status().State != StateError {
		t.Fatalf("hardlink accepted: %#v", p.Status())
	}
}

func TestWindowsAccessDeniedIsNotUnmanaged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(D;;FR;;;WD)(A;;FA;;;OW)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	if p := loadPath(path, testCatalog()); p.Status().State != StateError {
		t.Fatalf("access denial treated as absence: %#v", p.Status())
	}
}
