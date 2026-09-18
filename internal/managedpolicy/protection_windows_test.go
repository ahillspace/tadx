package managedpolicy

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsSecurityDescriptors(t *testing.T) {
	for _, tc := range []struct {
		name, sddl          string
		file, parent, valid bool
	}{
		{"protected", "O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FR;;;BU)", true, false, true},
		{"trusted-installer", "O:S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464D:P(A;;FA;;;SY)(A;;FR;;;BU)", true, false, true},
		{"user-owner", "O:BUG:BUD:P(A;;FR;;;BU)", true, false, false},
		{"null-dacl", "O:BAG:BA", true, false, false},
		{"user-write", "O:BAG:BAD:P(A;;FW;;;BU)", true, false, false},
		{"write-dacl", "O:BAG:BAD:P(A;;WD;;;BU)", true, false, false},
		{"write-owner", "O:BAG:BAD:P(A;;WO;;;BU)", true, false, false},
		{"delete-child", "O:BAG:BAD:P(A;;0x40;;;BU)", false, false, false},
		{"inherited-only", "O:BAG:BAD:P(A;IO;FA;;;BU)", false, false, true},
		{"ancestor-create", "O:BAG:BAD:P(A;;0x4;;;BU)", false, false, true},
		{"parent-create", "O:BAG:BAD:P(A;;0x4;;;BU)", false, true, false},
		{"deny-does-not-hide-grant", "O:BAG:BAD:P(D;;FW;;;BU)(A;;FW;;;BU)", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString(tc.sddl)
			if err != nil {
				t.Fatal(err)
			}
			if err := checkSecurityDescriptor(sd, tc.file, tc.parent); (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}

func TestWindowsSystemPathIgnoresEnvironment(t *testing.T) {
	before, err := SystemPath()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ProgramFiles", t.TempDir())
	t.Setenv("PROGRAMW6432", t.TempDir())
	t.Setenv("TADX_MANAGED_POLICY", filepath.Join(t.TempDir(), "policy.json"))
	after, err := SystemPath()
	if err != nil || before != after {
		t.Fatalf("discovery changed: %q %q %v", before, after, err)
	}
}

func TestWindowsUserOwnedCandidateDoesNotActivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed-policy.json")
	data := []byte(`{"version":1,"allowed_capabilities":["read"],"remote_mutations":false}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(data, testCatalog()); err != nil {
		t.Fatal(err)
	}
	p := loadPath(path, testCatalog())
	if p.Status().State != StateError || p.Status().Protected {
		t.Fatalf("unprotected candidate activated: %#v", p.Status())
	}
}

func TestWindowsMissingPolicyAndLinks(t *testing.T) {
	dir := t.TempDir()
	if p := loadPath(filepath.Join(dir, "absent.json"), testCatalog()); p.Status().State != StateUnmanaged {
		t.Fatalf("missing policy: %#v", p.Status())
	}
	ordinary := filepath.Join(dir, "ordinary-file")
	if err := os.WriteFile(ordinary, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if p := loadPath(filepath.Join(ordinary, "absent.json"), testCatalog()); p.Status().State != StateError {
		t.Fatalf("non-directory treated as absent: %#v", p.Status())
	}
	link := filepath.Join(dir, "dangling-link")
	if err := os.Symlink(filepath.Join(dir, "missing"), link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if p := loadPath(link, testCatalog()); p.Status().State != StateError {
		t.Fatalf("dangling reparse point treated as absent: %#v", p.Status())
	}
}
