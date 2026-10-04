package version_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ahillspace/tadx/actions/version"
	versioncore "github.com/ahillspace/tadx/internal/version"
)

type checker struct{ calls int }

func (c *checker) Latest(context.Context) (versioncore.Release, error) {
	c.calls++
	return versioncore.Release{Version: "1.2.0", URL: "https://github.com/ahillspace/tadx/releases/tag/v1.2.0", PublishedAt: time.Now()}, nil
}
func TestOfflineByDefault(t *testing.T) {
	c := &checker{}
	out, err := version.New("1.1.0", c).Get(t.Context(), version.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if c.calls != 0 || out.Version != "1.1.0" || out.Status != "installed" {
		t.Fatalf("out=%#v calls=%d", out, c.calls)
	}
}

func TestCheckReportsCurrentVersion(t *testing.T) {
	c := &checker{}
	out, err := version.New("1.2.0", c).Get(t.Context(), version.Input{Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if c.calls != 1 || out.UpdateAvailable || out.Status != "current" || len(out.Help) != 0 {
		t.Fatalf("out=%#v calls=%d", out, c.calls)
	}
}
func TestCheckReportsUpdate(t *testing.T) {
	c := &checker{}
	out, err := version.New("1.1.0", c).Get(t.Context(), version.Input{Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if c.calls != 1 || !out.UpdateAvailable || out.Status != "update-available" || !reflect.DeepEqual(out.Help, []string{"tadx update"}) {
		t.Fatalf("out=%#v calls=%d", out, c.calls)
	}
}

func TestCheckDoesNotClaimUpdateForDevelopmentVersion(t *testing.T) {
	c := &checker{}
	out, err := version.New("dev", c).Get(t.Context(), version.Input{Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if c.calls != 1 || out.Version != "dev" || out.LatestVersion != "1.2.0" || out.UpdateAvailable || out.Status != "current" {
		t.Fatalf("out=%#v calls=%d", out, c.calls)
	}
}
