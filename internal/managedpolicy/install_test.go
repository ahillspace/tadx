package managedpolicy

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
)

type fixtureInstaller struct {
	events []string
	fail   string
	active string
}

func (f *fixtureInstaller) step(name string) error {
	f.events = append(f.events, name)
	if f.fail == name {
		return errors.New("injected " + name)
	}
	return nil
}
func (f *fixtureInstaller) lock(context.Context) (func(), error) { return func() {}, f.step("lock") }
func (f *fixtureInstaller) current() (string, error)             { return f.active, nil }
func (f *fixtureInstaller) prepare(string) (func(), bool, error) {
	return func() {}, true, f.step("prepare")
}
func (f *fixtureInstaller) write(string, []byte) error { return f.step("write") }
func (f *fixtureInstaller) verify(string) error        { return f.step("verify") }
func (f *fixtureInstaller) publish(string) (bool, error) {
	err := f.step("locator")
	return err == nil, err
}

func TestInstallPublishesOnlyAfterProtectedPolicy(t *testing.T) {
	f := &fixtureInstaller{}
	out, err := installWith(t.Context(), InstallResult{Path: "selected/managed-policy.json", Template: "admin"}, []byte("policy"), f)
	if err != nil || !out.PolicyWritten || !out.LocatorPublished || !out.Active {
		t.Fatalf("receipt=%+v error=%v", out, err)
	}
	if want := []string{"lock", "prepare", "write", "verify", "locator"}; !reflect.DeepEqual(f.events, want) {
		t.Fatalf("events=%v", f.events)
	}
}

func TestInstallFailureReceipts(t *testing.T) {
	for _, fail := range []string{"prepare", "write", "verify", "locator"} {
		for _, alreadyActive := range []bool{false, true} {
			t.Run(fail+"/"+map[bool]string{true: "active", false: "inactive"}[alreadyActive], func(t *testing.T) {
				f := &fixtureInstaller{fail: fail}
				if alreadyActive {
					f.active = "selected/managed-policy.json"
				}
				out, err := installWith(t.Context(), InstallResult{Path: "selected/managed-policy.json", Template: "admin"}, nil, f)
				written := fail == "verify" || fail == "locator"
				if err == nil || out.Phase != fail || out.PolicyWritten != written || out.LocatorPublished || out.Active != (written && alreadyActive) {
					t.Fatalf("receipt=%+v error=%v", out, err)
				}
			})
		}
	}
}

func TestInstallCancelledBeforeMutation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	f := &fixtureInstaller{}
	out, err := installWith(ctx, InstallResult{}, nil, f)
	if !errors.Is(err, context.Canceled) || out.PolicyWritten || len(f.events) != 0 {
		t.Fatalf("receipt=%+v events=%v error=%v", out, f.events, err)
	}
}

func TestInstallationWarningsInspectConfirmedDestinationOnly(t *testing.T) {
	const destination = "chosen/managed-policy.json"
	called := 0
	read := func(path string) ([]byte, []ProtectionCheck, error) {
		called++
		if path != destination {
			t.Fatalf("inspected %q instead of destination", path)
		}
		return []byte("malformed installed JSON"), []ProtectionCheck{{Kind: "ancestor-owner-acl-and-links", Reason: "unprotected"}}, nil
	}
	for _, result := range []InstallResult{{Path: destination}, {PolicyWritten: true}} {
		if warnings := installationWarnings(result, read); len(warnings) != 0 {
			t.Fatalf("unconfirmed destination warnings=%v", warnings)
		}
	}
	if called != 0 {
		t.Fatalf("unconfirmed destination inspected %d times", called)
	}
	result := InstallResult{Path: destination, PolicyWritten: true, Phase: "locator"}
	if warnings := installationWarnings(result, read); len(warnings) != 1 || called != 1 {
		t.Fatalf("confirmed destination warnings=%v calls=%d", warnings, called)
	}
	if warnings := installationWarnings(result, read); len(warnings) != 1 || called != 2 {
		t.Fatalf("destination warning read was reused: warnings=%v calls=%d", warnings, called)
	}
}

func TestInstallationWarningsPreserveReadOutcomes(t *testing.T) {
	for _, test := range []struct {
		name     string
		err      error
		ancestor bool
		warnings int
	}{
		{"malformed document with ancestor warning", nil, true, 1},
		{"malformed document with protected ancestor", nil, false, 0},
		{"mandatory leaf failure retains ancestor warning", errors.New("unsafe policy file"), true, 1},
		{"mandatory leaf failure is not an ancestor warning", errors.New("unsafe policy file"), false, 0},
		{"missing destination suppresses earlier warnings", os.ErrNotExist, true, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := InstallResult{Path: "chosen/managed-policy.json", PolicyWritten: true, Phase: "verify"}
			checks := []ProtectionCheck{{Kind: "ancestor-owner-acl-and-links", Passed: !test.ancestor}, {Kind: "owner-acl-and-links", Passed: false}}
			warnings := installationWarnings(result, func(string) ([]byte, []ProtectionCheck, error) {
				return []byte("not JSON"), checks, test.err
			})
			if len(warnings) != test.warnings || result.Active || result.LocatorPublished || result.Phase != "verify" {
				t.Fatalf("warnings=%v receipt=%+v", warnings, result)
			}
		})
	}
}
