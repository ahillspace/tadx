package profile_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	profileadd "github.com/ahillspace/tadx/actions/env/profile"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type addTestStore struct {
	got    profileadd.AddProfile
	result profileadd.AddProfile
	err    error
}

func (a *addTestStore) Add(_ context.Context, profile profileadd.AddProfile) (profileadd.AddProfile, error) {
	a.got = profile
	return a.result, a.err
}

func (a *addTestStore) PreviewAdd(ctx context.Context, profile profileadd.AddProfile) (profileadd.AddProfile, error) {
	return a.Add(ctx, profile)
}

func TestAddExecuteValidatesRequiredInputBeforeWriting(t *testing.T) {
	store := &addTestStore{}
	for _, input := range []profileadd.AddInput{{}, {Alias: "production"}, {Alias: "production", ServerURL: "http://example.test"}} {
		_, err := profileadd.NewAdd(store).Execute(context.Background(), input)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
			t.Fatalf("input %#v error = %#v", input, err)
		}
	}
	if store.got.Alias != "" {
		t.Fatalf("store called with %#v", store.got)
	}
}

func TestAddExecuteAddsPATProfile(t *testing.T) {
	store := &addTestStore{result: addFixture()}
	got, err := profileadd.NewAdd(store).Execute(context.Background(), profileadd.AddInput{Alias: "Prod-West", ServerURL: "https://example.test", SiteContentURL: "marketing"})
	if err != nil || got.Status != "added" || store.got.AuthType != "pat" || store.got.Alias != "Prod-West" {
		t.Fatalf("output = %#v, input = %#v, error = %v", got, store.got, err)
	}
}

func TestAddCacheConcurrencyValidatedAndPassedToStore(t *testing.T) {
	for _, limit := range []int{-1, 0, 1, 256, 257} {
		store := &addTestStore{}
		_, err := profileadd.NewAdd(store).Execute(context.Background(), profileadd.AddInput{Alias: "staging", ServerURL: "https://tableau.example.com", CacheMaxConcurrency: limit})
		if limit < 0 || limit > 256 {
			if err == nil || store.got.Alias != "" {
				t.Fatalf("invalid%d error=%v stored=%+v", limit, err, store.got)
			}
		} else if err != nil || store.got.CacheMaxConcurrency != limit {
			t.Fatalf("limit%d error=%v stored=%+v", limit, err, store.got)
		}
	}
}

func TestAddOutputGoldens(t *testing.T) {
	got, _ := profileadd.NewAdd(&addTestStore{result: addFixture()}).Execute(context.Background(), profileadd.AddInput{Alias: "production", ServerURL: "https://example.test"})
	addAssertGolden(t, got, false, "testdata/add/output.toon")
	addAssertGolden(t, got, true, "testdata/add/output_full.toon")
}

func addFixture() profileadd.AddProfile {
	return profileadd.AddProfile{Alias: "production", ServerURL: "https://example.test", SiteContentURL: "marketing", APIVersion: "3.29", AuthType: "pat", PATNameEnv: "PROD_PAT_NAME", PATSecretEnv: "PROD_PAT_SECRET", DefaultWorkspace: "primary"}
}
func addAssertGolden(t *testing.T, value any, full bool, path string) {
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
