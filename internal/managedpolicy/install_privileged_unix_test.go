//go:build linux || darwin

package managedpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func unixInstallFixture(t *testing.T) (string, string, string) {
	t.Helper()
	if os.Geteuid() != 0 {
		if os.Getenv("TADX_TEST_REQUIRE_PRIVILEGED_POLICY") == "1" {
			t.Fatal("dedicated native policy job requires root")
		}
		t.Skip("requires root for an isolated protected fixture")
	}
	base := "/root"
	if runtime.GOOS == "darwin" {
		base = "/private/var/root"
	}
	dir, err := os.MkdirTemp(base, "tadx-install-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	return dir, filepath.Join(dir, "location.json"), filepath.Join(dir, "default", "managed-policy.json")
}

func TestPrivilegedUnixInstallLifecycle(t *testing.T) {
	base, locator, defaultPath := unixInstallFixture(t)
	first, second := filepath.Join(base, "first"), filepath.Join(base, "second")
	for _, step := range []struct {
		directory, template string
		remote              bool
	}{
		{first, "read-only", false}, {second, "read-write-no-admin", true}, {second, "read-only", false}, {"", "", true},
	} {
		out, err := runUnixInstall(t.Context(), InstallOptions{Directory: step.directory, Template: step.template}, testCatalog(), true, nil, locator, defaultPath)
		if err != nil || out.Phase != "complete" || !out.PolicyWritten || !out.LocatorPublished || !out.Active {
			t.Fatalf("install=%+v error=%v", out, err)
		}
		path, required, err := readUnixLocation(locator, defaultPath)
		if err != nil || !required || path != out.Path {
			t.Fatalf("location=%q required=%t error=%v", path, required, err)
		}
		status := loadResolved(path, required, err, testCatalog()).Status()
		if status.State != StateActive || !status.Protected || !status.PathProtected || status.RemoteMutations != step.remote {
			t.Fatalf("status=%+v", status)
		}
	}
	for _, dir := range []string{first, second} {
		if _, err := os.Stat(filepath.Join(dir, "managed-policy.json")); err != nil {
			t.Fatalf("old destination was removed: %v", err)
		}
	}
	// An administrator's hand-edited policy remains valid without reinstalling.
	if err := os.WriteFile(defaultPath, []byte(`{"version":1,"allowed_capabilities":["read"],"remote_mutations":false}`), 0644); err != nil {
		t.Fatal(err)
	}
	if status := loadPath(defaultPath, testCatalog()).Status(); status.State != StateActive || status.RemoteMutations {
		t.Fatalf("hand-edited policy=%+v", status)
	}
	if err := os.WriteFile(locator, []byte("broken locator"), 0644); err != nil {
		t.Fatal(err)
	}
	path, required, err := readUnixLocation(locator, defaultPath)
	if status := loadResolved(path, required, err, testCatalog()).Status(); status.State != StateError {
		t.Fatalf("malformed locator=%+v", status)
	}
	out, err := runUnixInstall(t.Context(), InstallOptions{}, testCatalog(), true, nil, locator, defaultPath)
	if err != nil || !out.Active || out.Path != defaultPath {
		t.Fatalf("recovery=%+v error=%v", out, err)
	}
	if err := os.Remove(defaultPath); err != nil {
		t.Fatal(err)
	}
	path, required, err = readUnixLocation(locator, defaultPath)
	if status := loadResolved(path, required, err, testCatalog()).Status(); status.State != StateError {
		t.Fatalf("missing installed policy=%+v", status)
	}
}

func TestPrivilegedUnixInstallRejectsUntrustedPaths(t *testing.T) {
	for _, kind := range []string{"ancestor-write", "ancestor-owner", "ancestor-symlink", "leaf-write", "leaf-owner", "leaf-symlink", "policy-write", "policy-owner", "policy-symlink", "policy-hardlink", "unrelated", "missing-parent"} {
		t.Run(kind, func(t *testing.T) {
			base, locator, defaultPath := unixInstallFixture(t)
			parent := filepath.Join(base, "parent")
			dir := filepath.Join(parent, "policy")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "managed-policy.json")
			if err := os.WriteFile(path, []byte("original policy"), 0644); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "ancestor-write":
				err = os.Chmod(parent, 0777)
			case "ancestor-owner":
				err = os.Chown(parent, 65534, 65534)
			case "leaf-write":
				err = os.Chmod(dir, 0777)
			case "leaf-owner":
				err = os.Chown(dir, 65534, 65534)
			case "policy-write":
				err = os.Chmod(path, 0666)
			case "policy-owner":
				err = os.Chown(path, 65534, 65534)
			case "ancestor-symlink":
				link := filepath.Join(base, "parent-link")
				err = os.Symlink(parent, link)
				dir = filepath.Join(link, "policy")
			case "leaf-symlink":
				link := filepath.Join(parent, "policy-link")
				err = os.Symlink(dir, link)
				dir = link
			case "policy-symlink":
				if err := os.Rename(path, filepath.Join(base, "original")); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(filepath.Join(base, "original"), path)
			case "policy-hardlink":
				err = os.Link(path, filepath.Join(base, "original"))
			case "unrelated":
				err = os.WriteFile(filepath.Join(dir, "unrelated"), []byte("keep"), 0644)
			case "missing-parent":
				dir = filepath.Join(base, "missing", "policy")
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(parent)
			if err != nil {
				t.Fatal(err)
			}
			out, err := runUnixInstall(t.Context(), InstallOptions{Directory: dir}, testCatalog(), true, nil, locator, defaultPath)
			if err == nil || out.PolicyWritten || out.LocatorPublished {
				t.Fatalf("unsafe path accepted: %+v error=%v", out, err)
			}
			after, err := os.Stat(parent)
			if err != nil || before.Mode() != after.Mode() {
				t.Fatalf("ancestor changed: %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "original policy" {
				t.Fatalf("existing policy modified: %q error=%v", data, err)
			}
		})
	}
}

func TestPrivilegedUnixInstallUnsafeLocatorPreservesPartialReceipt(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "writable", "owner"} {
		t.Run(kind, func(t *testing.T) {
			base, locator, defaultPath := unixInstallFixture(t)
			target := filepath.Join(base, "untouched")
			if err := os.WriteFile(target, []byte("keep"), 0644); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(target, locator)
			case "hardlink":
				err = os.Link(target, locator)
			default:
				err = os.WriteFile(locator, []byte("keep"), 0644)
				if err == nil && kind == "writable" {
					err = os.Chmod(locator, 0666)
				}
				if err == nil && kind == "owner" {
					err = os.Chown(locator, 65534, 65534)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			out, err := runUnixInstall(t.Context(), InstallOptions{}, testCatalog(), true, nil, locator, defaultPath)
			if err == nil || !out.PolicyWritten || out.LocatorPublished || out.Active || out.Phase != "locator" {
				t.Fatalf("receipt=%+v error=%v", out, err)
			}
			if data, err := os.ReadFile(target); err != nil || string(data) != "keep" {
				t.Fatalf("locator target changed: %q %v", data, err)
			}
		})
	}
}

func TestPrivilegedUnixNoTerminalOrPrivilege(t *testing.T) {
	if os.Getenv("TADX_UNIX_INSTALL_CHILD") == "1" {
		if os.Geteuid() == 0 {
			t.Fatal("child must be unprivileged")
		}
		out, err := Install(t.Context(), InstallOptions{}, testCatalog())
		if err == nil || out.Phase != "elevation" || !strings.Contains(err.Error(), "sudo tadx policy install") {
			t.Fatalf("nonterminal result=%+v error=%v", out, err)
		}
		if handled, code := RunInstallHelper([]string{installHelperArgument, "/never-write", "superuser"}, testCatalog()); !handled || code == 0 {
			t.Fatal("unprivileged helper accepted")
		}
		return
	}
	unixInstallFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The CI test binary may be beneath a private runner home. Copy only this
	// executable into an isolated world-traversable temporary fixture for nobody.
	base := "/tmp"
	if runtime.GOOS == "darwin" {
		base = "/private/tmp"
	}
	dir, err := os.MkdirTemp(base, "tadx-unprivileged-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(dir, "policy.test")
	if err := os.WriteFile(child, data, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(child, "-test.run=^TestPrivilegedUnixNoTerminalOrPrivilege$")
	cmd.Env = append(os.Environ(), "TADX_UNIX_INSTALL_CHILD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("unprivileged child: %v\n%s", err, output)
	}
}

func TestUnixInstallValidationAndElevationFailure(t *testing.T) {
	for _, directory := range []string{"relative", "/", "/root/../bad", "/bad/", "/bad\x00", "/root", "/home", filepath.Join("/home", "fixture-user"), filepath.Join("/Users", "fixture-user"), "/Library/Application Support"} {
		out, _, err := prepareUnixInstall(InstallOptions{Directory: directory}, testCatalog(), defaultUnixPolicyPath())
		if err == nil || out.Phase != "validation" {
			t.Fatalf("accepted directory %q", directory)
		}
	}
	cause := errors.New("sudo denied")
	elevate := func(_ context.Context, out InstallResult) (InstallResult, error) {
		out.Phase = "elevation"
		return out, cause
	}
	out, err := runUnixInstall(t.Context(), InstallOptions{}, testCatalog(), false, elevate, "/unused", defaultUnixPolicyPath())
	if !errors.Is(err, cause) || out.PolicyWritten || out.Template != "superuser" {
		t.Fatalf("receipt=%+v error=%v", out, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := runUnixInstall(ctx, InstallOptions{}, testCatalog(), false, elevate, "/unused", defaultUnixPolicyPath()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
}

func TestUnixInstallReceiptBoundary(t *testing.T) {
	expected := InstallResult{Path: "/protected/managed-policy.json", Template: "superuser"}
	partial := expected
	partial.Phase, partial.PolicyWritten, partial.Active = "verify", true, true
	data, err := json.Marshal(unixInstallReceipt{Result: partial, Error: "verification failed"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := decodeUnixInstallReceipt(expected, data, false, errors.New("exit 1"))
	if err == nil || out != partial {
		t.Fatalf("partial=%+v error=%v", out, err)
	}
	complete := expected
	complete.Phase, complete.PolicyWritten, complete.LocatorPublished, complete.Active = "complete", true, true, true
	data, err = json.Marshal(unixInstallReceipt{Result: complete})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := decodeUnixInstallReceipt(expected, data, false, nil); err != nil || out != complete {
		t.Fatalf("complete=%+v error=%v", out, err)
	}
	if out, err := decodeUnixInstallReceipt(expected, data, true, nil); err == nil || out.Phase != "unknown" {
		t.Fatalf("overflow=%+v error=%v", out, err)
	}
	for _, input := range [][]byte{nil, []byte("garbage"), []byte(`{"result":{"path":"/other"}}`)} {
		out, err := decodeUnixInstallReceipt(expected, input, false, errors.New("exit 1"))
		if err == nil || out.Phase != "unknown" || out.PolicyWritten {
			t.Fatalf("unknown=%+v error=%v", out, err)
		}
	}
	var bounded boundedReceipt
	data = make([]byte, maxInstallReceipt+100)
	if n, err := bounded.Write(data); err != nil || n != len(data) || !bounded.overflow || bounded.Len() != maxInstallReceipt {
		t.Fatalf("unbounded receipt n=%d error=%v", n, err)
	}
}

func TestPrivilegedUnixInstallLockCancellationAndProtection(t *testing.T) {
	base, locator, defaultPath := unixInstallFixture(t)
	installer := &unixInstaller{locator: locator, defaultPath: defaultPath}
	unlock, err := installer.lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := installer.lock(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("lock cancellation: %v", err)
	}
	lock := filepath.Join(base, ".tadx-policy-install.lock")
	if err := os.Chmod(lock, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := installer.lock(t.Context()); err == nil {
		t.Fatal("unsafe lock accepted")
	}
}

func TestPrivilegedUnixLocatorStrictParsing(t *testing.T) {
	_, locator, defaultPath := unixInstallFixture(t)
	path, required, err := readUnixLocation(locator, defaultPath)
	if err != nil || required || path != defaultPath {
		t.Fatalf("missing locator=%q %t %v", path, required, err)
	}
	for _, data := range []string{`null`, `1`, `{}`, `"relative"`, `"/"`, `"/etc/../other"`, `"/valid" "trailing"`} {
		if err := os.WriteFile(locator, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
		if _, required, err := readUnixLocation(locator, defaultPath); err == nil || !required {
			t.Fatalf("invalid locator accepted: %s", data)
		}
	}
}
