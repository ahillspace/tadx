package profile_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	profileget "github.com/ahillspace/tadx/actions/env/profile"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type getTestReader struct {
	profile profileget.Profile
	err     error
	alias   *string
}

func (r getTestReader) Get(_ context.Context, alias string) (profileget.Profile, error) {
	if r.alias != nil {
		*r.alias = alias
	}
	return r.profile, r.err
}

func TestGetExecuteRequiresExactAliasAndReturnsProfile(t *testing.T) {
	action := profileget.NewGet(getTestReader{profile: getFixture()})
	if _, err := action.Execute(context.Background(), profileget.GetInput{}); err == nil {
		t.Fatal("empty alias accepted")
	}
	got, err := action.Execute(context.Background(), profileget.GetInput{Alias: "production"})
	if err != nil || got.Profile.Alias != "production" {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestGetExecutePreservesExactAlias(t *testing.T) {
	var gotAlias string
	_, err := profileget.NewGet(getTestReader{profile: getFixture(), alias: &gotAlias}).Execute(context.Background(), profileget.GetInput{Alias: "Prod-West"})
	if err != nil {
		t.Fatal(err)
	}
	if gotAlias != "Prod-West" {
		t.Fatalf("alias = %q", gotAlias)
	}
}

func TestGetExecuteWrapsReadFailure(t *testing.T) {
	_, err := profileget.NewGet(getTestReader{err: errors.New("missing")}).Execute(context.Background(), profileget.GetInput{Alias: "production"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "env.profile.get.read" {
		t.Fatalf("error = %#v", err)
	}
}

func TestGetInvalidProfileShowsFieldsAndRepairCommands(t *testing.T) {
	profile := profileget.Profile{Alias: "broken", Default: true, Status: "invalid", Violations: []string{"pat_secret_env"}}
	got, err := profileget.NewGet(getTestReader{profile: profile}).Execute(t.Context(), profileget.GetInput{Alias: "broken"})
	if err != nil || got.Profile.Status != "invalid" || len(got.Help) != 2 || !strings.Contains(got.Help[0], "tadx env update broken") || !strings.Contains(got.Help[1], "tadx env remove broken") {
		t.Fatalf("invalid profile lost recovery: got=%+v err=%v", got, err)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"\"pat_secret_env\":", "\"server_url\":", "\"auth_type\":"} {
		if strings.Contains(string(data), unwanted) {
			t.Fatalf("invalid profile rendered an entry value: %s", data)
		}
	}
}

func TestGetOutputGoldens(t *testing.T) {
	got, _ := profileget.NewGet(getTestReader{profile: getFixture()}).Execute(context.Background(), profileget.GetInput{Alias: "production"})
	getAssertGolden(t, got, false, "testdata/get/output.toon")
	getAssertGolden(t, got, true, "testdata/get/output_full.toon")
	if got.CompactOutput().(profileget.GetCompactResult).Details != "--full" {
		t.Fatal("compact output omitted details marker")
	}
}

func getFixture() profileget.Profile {
	return profileget.Profile{Alias: "production", Default: true, ServerURL: "https://example.test", SiteContentURL: "marketing", AuthType: "pat", PATNameEnv: "PROD_PAT_NAME", PATSecretEnv: "PROD_PAT_SECRET", DefaultWorkspace: "primary"}
}
func getAssertGolden(t *testing.T, value any, full bool, path string) {
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
