package get_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	profileget "github.com/ahillspace/tadx/actions/env/profile/get"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type reader struct {
	profile profileget.Profile
	err     error
	alias   *string
}

func (r reader) Get(_ context.Context, alias string) (profileget.Profile, error) {
	if r.alias != nil {
		*r.alias = alias
	}
	return r.profile, r.err
}

func TestExecuteRequiresExactAliasAndReturnsProfile(t *testing.T) {
	action := profileget.New(reader{profile: fixture()})
	if _, err := action.Execute(context.Background(), profileget.Input{}); err == nil {
		t.Fatal("empty alias accepted")
	}
	got, err := action.Execute(context.Background(), profileget.Input{Alias: "production"})
	if err != nil || got.Profile.Alias != "production" {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestExecutePreservesExactAlias(t *testing.T) {
	var gotAlias string
	_, err := profileget.New(reader{profile: fixture(), alias: &gotAlias}).Execute(context.Background(), profileget.Input{Alias: "Prod-West"})
	if err != nil {
		t.Fatal(err)
	}
	if gotAlias != "Prod-West" {
		t.Fatalf("alias = %q", gotAlias)
	}
}

func TestExecuteWrapsReadFailure(t *testing.T) {
	_, err := profileget.New(reader{err: errors.New("missing")}).Execute(context.Background(), profileget.Input{Alias: "production"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "env.profile.get.read" {
		t.Fatalf("error = %#v", err)
	}
}

func TestOutputGoldens(t *testing.T) {
	got, _ := profileget.New(reader{profile: fixture()}).Execute(context.Background(), profileget.Input{Alias: "production"})
	assertGolden(t, got, false, "testdata/output.toon")
	assertGolden(t, got, true, "testdata/output_full.toon")
	if got.CompactOutput().(profileget.CompactResult).Details != "--full" {
		t.Fatal("compact output omitted details marker")
	}
}

func fixture() profileget.Profile {
	return profileget.Profile{Alias: "production", Default: true, ServerURL: "https://example.test", SiteContentURL: "marketing", APIVersion: "3.29", AuthType: "pat", PATNameEnv: "PROD_PAT_NAME", PATSecretEnv: "PROD_PAT_SECRET", DefaultWorkspace: "primary"}
}
func assertGolden(t *testing.T, value any, full bool, path string) {
	t.Helper()
	var b bytes.Buffer
	if err := output.RenderWithOptions(&b, value, output.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Bytes(), want) {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, b.Bytes())
	}
}
