package managedpolicy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsInstallRequestsElevationWithoutWriting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "policy")
	called := false
	out, err := runWindowsInstall(t.Context(), InstallOptions{Directory: dir}, testCatalog(), false, func(_ context.Context, out InstallResult) (InstallResult, error) {
		called = true
		if out.Template != "superuser" || out.Path != filepath.Join(dir, "managed-policy.json") {
			t.Fatalf("request=%+v", out)
		}
		return out, errors.New("approval cancelled")
	})
	if !called || err == nil || out.PolicyWritten || out.ProtectionChanged {
		t.Fatalf("receipt=%+v err=%v", out, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination changed before approval: %v", err)
	}
}

func TestWindowsInstallDefaultsAndValidation(t *testing.T) {
	defaultPath, err := defaultSystemPath()
	if err != nil {
		t.Fatal(err)
	}
	out, data, err := prepareInstall(InstallOptions{}, testCatalog())
	if err != nil || out.Path != defaultPath || out.Template != "superuser" {
		t.Fatalf("receipt=%+v err=%v", out, err)
	}
	doc, err := Parse(data, testCatalog())
	if err != nil || !doc.RemoteMutations || len(doc.AllowedCapabilities) != len(testCatalog()) {
		t.Fatalf("document=%+v err=%v", doc, err)
	}
	for _, dir := range []string{"relative", `C:\`, `\\server\share\policy`, `C:\policy.`, `C:\policy:stream`, `C:\policy\..\other`} {
		if _, _, err := prepareInstall(InstallOptions{Directory: dir}, testCatalog()); err == nil {
			t.Errorf("accepted %q", dir)
		}
	}
	home, err := windows.KnownFolderPath(windows.FOLDERID_Profile, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareInstall(InstallOptions{Directory: home}, testCatalog()); err == nil {
		t.Fatal("accepted user home")
	}
	if _, _, err := prepareInstall(InstallOptions{Template: "unknown"}, testCatalog()); err == nil {
		t.Fatal("accepted unknown template")
	}
}

func TestWindowsInstallNormalizesLegacyTemplate(t *testing.T) {
	out, data, err := prepareInstall(InstallOptions{Template: "admin"}, testCatalog())
	if err != nil || out.Template != "superuser" {
		t.Fatalf("receipt=%+v err=%v", out, err)
	}
	doc, err := Parse(data, testCatalog())
	if err != nil || !doc.RemoteMutations || len(doc.AllowedCapabilities) != len(testCatalog()) {
		t.Fatalf("document=%+v err=%v", doc, err)
	}
}

func TestWindowsElevationReceiptRoundTrip(t *testing.T) {
	for _, phase := range installPhases {
		for flags := range 16 {
			want := InstallResult{Path: "chosen", Template: "admin", Phase: phase, PolicyWritten: flags&1 != 0, LocatorPublished: flags&2 != 0, Active: flags&4 != 0, ProtectionChanged: flags&8 != 0}
			got, err := decodeInstallExit(InstallResult{Path: want.Path, Template: want.Template}, uint32(encodeInstallExit(want, errors.New("failure"))))
			if err == nil || got != want {
				t.Fatalf("receipt mismatch: got=%+v want=%+v err=%v", got, want, err)
			}
		}
	}
	out, err := decodeInstallExit(InstallResult{}, 1)
	if err == nil || out.Phase != "unknown" {
		t.Fatalf("crash receipt=%+v err=%v", out, err)
	}
}

func TestWindowsElevationReceiptPreservesActionableCause(t *testing.T) {
	for _, cause := range []error{windows.ERROR_ACCESS_DENIED, errors.New(installFailureReasons[5])} {
		_, err := decodeInstallExit(InstallResult{}, uint32(encodeInstallExit(InstallResult{Phase: "prepare"}, cause)))
		if err == nil || !strings.Contains(err.Error(), cause.Error()) {
			t.Fatalf("cause=%v receipt=%v", cause, err)
		}
	}
}

func TestWindowsAncestorChecksInspectACLs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "managed-policy.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	_, checks, _ := secureRead(path)
	if len(checks) < 3 {
		t.Fatalf("checks=%+v", checks)
	}
	for _, check := range checks[:len(checks)-2] {
		if check.Kind != "ancestor-owner-acl-and-links" {
			t.Fatalf("ancestor ACL not inspected: %+v", check)
		}
	}
}
