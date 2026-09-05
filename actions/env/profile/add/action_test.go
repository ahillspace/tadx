package add_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	profileadd "github.com/ahillspace/tadx/actions/env/profile/add"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type adder struct {
	got    profileadd.Profile
	result profileadd.Profile
	err    error
}

func (a *adder) Add(_ context.Context, profile profileadd.Profile) (profileadd.Profile, error) {
	a.got = profile
	return a.result, a.err
}

func TestExecuteValidatesRequiredInputBeforeWriting(t *testing.T) {
	store := &adder{}
	for _, input := range []profileadd.Input{{}, {Alias: "production"}, {Alias: "production", ServerURL: "http://example.test"}} {
		_, err := profileadd.New(store).Execute(context.Background(), input)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
			t.Fatalf("input %#v error = %#v", input, err)
		}
	}
	if store.got.Alias != "" {
		t.Fatalf("store called with %#v", store.got)
	}
}

func TestExecuteAddsPATProfile(t *testing.T) {
	store := &adder{result: fixture()}
	got, err := profileadd.New(store).Execute(context.Background(), profileadd.Input{Alias: "Prod-West", ServerURL: "https://example.test", SiteContentURL: "marketing"})
	if err != nil || got.Status != "added" || store.got.AuthType != "pat" || store.got.Alias != "Prod-West" {
		t.Fatalf("output = %#v, input = %#v, error = %v", got, store.got, err)
	}
}

func TestOutputGoldens(t *testing.T) {
	got, _ := profileadd.New(&adder{result: fixture()}).Execute(context.Background(), profileadd.Input{Alias: "production", ServerURL: "https://example.test"})
	assertGolden(t, got, false, "testdata/output.toon")
	assertGolden(t, got, true, "testdata/output_full.toon")
}

func fixture() profileadd.Profile {
	return profileadd.Profile{Alias: "production", ServerURL: "https://example.test", SiteContentURL: "marketing", APIVersion: "3.29", AuthType: "pat", PATNameEnv: "PROD_PAT_NAME", PATSecretEnv: "PROD_PAT_SECRET", DefaultWorkspace: "primary"}
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
