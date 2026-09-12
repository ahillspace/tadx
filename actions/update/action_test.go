package update

import (
	"context"
	"errors"
	"testing"
)

type fake struct {
	calls  int
	fail   bool
	latest string
}

func (*fake) Current() string                { return "1.0.0" }
func (*fake) ValidateTargets([]string) error { return nil }
func (f *fake) Latest(context.Context) (Release, error) {
	if f.latest != "" {
		return Release{Version: f.latest}, nil
	}
	return Release{Version: "1.1.0"}, nil
}
func (f *fake) Install(context.Context, Release, []string) error {
	f.calls++
	if f.fail {
		return errors.New("Guidance failed")
	}
	return nil
}
func TestCheckDoesNotInstall(t *testing.T) {
	f := &fake{}
	out, err := New(f).Execute(context.Background(), Input{Check: true})
	if err != nil || f.calls != 0 || out.Status != "checked" {
		t.Fatalf("%+v %v calls=%d", out, err, f.calls)
	}
}
func TestSameVersionStillRefreshesGuidance(t *testing.T) {
	f := &fake{latest: "1.0.0"}
	out, err := New(f).Execute(context.Background(), Input{})
	if err != nil || f.calls != 1 || out.Guidance != "refreshed" {
		t.Fatalf("%+v %v", out, err)
	}
}
func TestFailureDoesNotClaimSuccess(t *testing.T) {
	f := &fake{fail: true}
	out, err := New(f).Execute(context.Background(), Input{})
	if err == nil || out.Status == "updated" {
		t.Fatalf("%+v %v", out, err)
	}
}
