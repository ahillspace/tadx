package managedpolicy

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ahillspace/tadx/internal/capability"
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
	load := func(path string, _ []capability.Definition) *Policy {
		called++
		if path != destination {
			t.Fatalf("inspected %q instead of destination", path)
		}
		return &Policy{status: Status{Warnings: []string{"ancestor permits policy substitution"}}}
	}
	for _, result := range []InstallResult{{Path: destination}, {PolicyWritten: true}} {
		if warnings := installationWarnings(result, testCatalog(), load); len(warnings) != 0 {
			t.Fatalf("unconfirmed destination warnings=%v", warnings)
		}
	}
	if called != 0 {
		t.Fatalf("unconfirmed destination inspected %d times", called)
	}
	result := InstallResult{Path: destination, PolicyWritten: true, Phase: "locator"}
	if warnings := installationWarnings(result, testCatalog(), load); len(warnings) != 1 || called != 1 {
		t.Fatalf("confirmed destination warnings=%v calls=%d", warnings, called)
	}
}
