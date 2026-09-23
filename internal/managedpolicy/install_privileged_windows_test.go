package managedpolicy

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func isolatedWindowsInstaller(t *testing.T) *windowsInstaller {
	t.Helper()
	if !windows.GetCurrentProcessToken().IsElevated() {
		if os.Getenv("TADX_TEST_REQUIRE_PRIVILEGED_POLICY") == "1" {
			t.Fatal("dedicated native policy job requires an elevated administrator token")
		}
		t.Skip("requires an elevated native runner for an isolated protected installation")
	}
	parent := `SOFTWARE\TADX.PolicyInstall.Test.` + rand.Text()
	key := parent + `\ManagedPolicy`
	t.Cleanup(func() {
		software, err := registry.OpenKey(registry.LOCAL_MACHINE, "SOFTWARE", registry.ALL_ACCESS|registry.WOW64_64KEY)
		if err != nil {
			t.Error(err)
			return
		}
		defer software.Close()
		for _, path := range []string{filepath.Join(filepath.Base(parent), "ManagedPolicy"), filepath.Base(parent)} {
			if err := registry.DeleteKey(software, path); err != nil && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
				t.Error(err)
			}
		}
	})
	return &windowsInstaller{root: registry.LOCAL_MACHINE, key: key, definitions: testCatalog()}
}

func installFixture(t *testing.T, w *windowsInstaller, dir, template string) InstallResult {
	t.Helper()
	out, data, err := prepareInstall(InstallOptions{Directory: dir, Template: template}, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	out, err = installWith(t.Context(), out, data, w)
	if err != nil {
		t.Fatalf("install receipt=%+v error=%v", out, err)
	}
	return out
}

func TestPrivilegedWindowsInstallLocationsAndOverwrite(t *testing.T) {
	w := isolatedWindowsInstaller(t)
	base := t.TempDir()
	// Deliberately broad ancestor ACL, isolated from the machine policy and root.
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;BU)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(base, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	first := installFixture(t, w, filepath.Join(base, "first"), "read-only")
	if !first.PolicyWritten || !first.LocatorPublished || !first.Active {
		t.Fatalf("receipt=%+v", first)
	}
	policy := loadPath(first.Path, testCatalog())
	if policy.Status().State != StateActive || policy.Status().RemoteMutations {
		t.Fatalf("policy=%+v", policy.Status())
	}
	if policy.Status().PathProtected || len(policy.Status().Warnings) == 0 {
		t.Fatalf("unsafe ancestor was not reported: %+v", policy.Status())
	}
	if policy.CheckCapability("read") != nil || policy.CheckRemoteMutation() != ErrRemoteMutationDenied {
		t.Fatalf("ancestor warning changed effective policy: %+v", policy.Status())
	}
	for _, check := range policy.Status().Checks {
		if sameInstallPath(check.Path, base) && (check.Kind != "ancestor-owner-acl-and-links" || check.Passed) {
			t.Fatalf("ancestor ACL warning missing: %+v", check)
		}
	}
	installFixture(t, w, filepath.Dir(first.Path), "admin")
	if p := loadPath(first.Path, testCatalog()); !p.Status().RemoteMutations {
		t.Fatalf("overwrite did not take effect: %+v", p.Status())
	}
	previous, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	second := installFixture(t, w, filepath.Join(base, "second"), "read-only")
	current, required, err := readLocation(w.root, w.key)
	if err != nil || !required || !sameInstallPath(current, second.Path) {
		t.Fatalf("locator=%q required=%v err=%v", current, required, err)
	}
	still, err := os.ReadFile(first.Path)
	if err != nil || string(still) != string(previous) {
		t.Fatal("prior policy changed during location switch")
	}
	if err := os.Remove(second.Path); err != nil {
		t.Fatal(err)
	}
	path, required, err := readLocation(w.root, w.key)
	if p := loadResolved(path, required, err, testCatalog()); p.Status().State != StateError {
		t.Fatalf("dangling locator became unmanaged: %+v", p.Status())
	}
}

func TestPrivilegedWindowsLocatorFailsClosed(t *testing.T) {
	w := isolatedWindowsInstaller(t)
	path, required, err := readLocation(w.root, w.key)
	if err != nil || required || path == "" {
		t.Fatalf("absent locator: %q %v %v", path, required, err)
	}
	dir := filepath.Join(t.TempDir(), "policy")
	installFixture(t, w, dir, "admin")
	key, err := registry.OpenKey(w.root, w.key, registry.ALL_ACCESS|registry.WOW64_64KEY)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	if err := key.DeleteValue("Directory"); err != nil {
		t.Fatal(err)
	}
	if _, required, err := readLocation(w.root, w.key); err == nil || !required {
		t.Fatal("missing value accepted as absent locator")
	}
	if err := key.SetDWordValue("Directory", 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readLocation(w.root, w.key); err == nil {
		t.Fatal("wrong locator type accepted")
	}
	if err := key.SetStringValue("Directory", "relative"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readLocation(w.root, w.key); err == nil {
		t.Fatal("relative locator accepted")
	}
	// Repair through install remains possible with malformed current discovery.
	installFixture(t, w, dir, "read-only")
	if err := protectHandle(windows.Handle(key), windows.SE_REGISTRY_KEY, "O:BAG:BAD:P(A;CI;KA;;;SY)(A;CI;KA;;;BA)(A;CI;KA;;;BU)"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readLocation(w.root, w.key); err == nil {
		t.Fatal("unprotected locator accepted")
	}
	installFixture(t, w, dir, "admin")
}

func TestPrivilegedWindowsInstallRejectsUnrelatedContentsAndLinks(t *testing.T) {
	w := isolatedWindowsInstaller(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "unrelated.txt"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	out, data, err := prepareInstall(InstallOptions{Directory: dir}, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	result, err := installWith(t.Context(), out, data, w)
	if err == nil || result.ProtectionChanged || result.PolicyWritten {
		t.Fatalf("unrelated contents receipt=%+v err=%v", result, err)
	}
	base := t.TempDir()
	installed := installFixture(t, w, filepath.Join(base, "policy"), "admin")
	if err := os.Link(installed.Path, filepath.Join(base, "other-link")); err != nil {
		t.Fatal(err)
	}
	out, data, err = prepareInstall(InstallOptions{Directory: filepath.Dir(installed.Path)}, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	result, err = installWith(t.Context(), out, data, w)
	if err == nil || result.PolicyWritten {
		t.Fatalf("hardlink receipt=%+v err=%v", result, err)
	}
	link := filepath.Join(base, "linked")
	if err := os.Symlink(filepath.Dir(installed.Path), link); err != nil {
		t.Fatal(err)
	}
	out, data, err = prepareInstall(InstallOptions{Directory: link}, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	result, err = installWith(t.Context(), out, data, w)
	if err == nil || result.PolicyWritten {
		t.Fatalf("reparse receipt=%+v err=%v", result, err)
	}
}
