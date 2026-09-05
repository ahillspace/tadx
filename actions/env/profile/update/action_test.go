package update_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	profileupdate "github.com/ahillspace/tadx/actions/env/profile/update"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type updater struct {
	alias  string
	patch  profileupdate.Patch
	result profileupdate.UpdateResult
	err    error
}

func (u *updater) Update(_ context.Context, alias string, patch profileupdate.Patch) (profileupdate.UpdateResult, error) {
	u.alias, u.patch = alias, patch
	return u.result, u.err
}

type retryableStoreError struct{}

func (retryableStoreError) Error() string   { return "candidate rejected" }
func (retryableStoreError) Retryable() bool { return true }
func (retryableStoreError) CorrectiveAction() string {
	return "Correct the complete candidate profile."
}

func TestExecuteRequiresAliasAndExplicitFields(t *testing.T) {
	store := &updater{}
	for _, input := range []profileupdate.Input{{}, {Alias: "production"}} {
		_, err := profileupdate.New(store).Execute(context.Background(), input)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
			t.Fatalf("input %#v error = %#v", input, err)
		}
	}
}

func TestExecuteRejectsInvalidExplicitServerURLBeforeUpdater(t *testing.T) {
	store := &updater{}
	_, err := profileupdate.New(store).Execute(context.Background(), profileupdate.Input{Alias: "Prod-West", Patch: profileupdate.Patch{ServerURL: profileupdate.StringField{Set: true, Value: "http://example.test"}}})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error = %#v", err)
	}
	if store.alias != "" {
		t.Fatalf("updater called for invalid URL with alias %q", store.alias)
	}
}

func TestExecutePreservesExactAliasAndStructuredUpdaterAdvice(t *testing.T) {
	store := &updater{err: retryableStoreError{}}
	_, err := profileupdate.New(store).Execute(context.Background(), profileupdate.Input{Alias: "Prod-West", Patch: profileupdate.Patch{PATNameEnv: profileupdate.StringField{Set: true, Value: "NEW_NAME_REF"}}})
	payload := errs.Structure(err).Error
	if store.alias != "Prod-West" || payload.ID != "env.profile.update.write" || payload.Retryable == nil || !*payload.Retryable || payload.CorrectiveAction != "Correct the complete candidate profile." {
		t.Fatalf("alias = %q, error = %#v", store.alias, payload)
	}
}

func TestExecutePreservesDeterministicChangedFields(t *testing.T) {
	result := profileupdate.UpdateResult{Profile: fixture(), ChangedFields: []string{"api_version", "site_content_url", "api_version"}}
	store := &updater{result: result}
	patch := profileupdate.Patch{SiteContentURL: profileupdate.StringField{Set: true, Value: "marketing"}, APIVersion: profileupdate.StringField{Set: true, Value: "3.29"}}
	got, err := profileupdate.New(store).Execute(context.Background(), profileupdate.Input{Alias: "production", Patch: patch})
	if err != nil || got.Status != "updated" || !reflect.DeepEqual(got.ChangedFields, []string{"site_content_url", "api_version"}) {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestExecuteReturnsUnchanged(t *testing.T) {
	store := &updater{result: profileupdate.UpdateResult{Profile: fixture()}}
	got, err := profileupdate.New(store).Execute(context.Background(), profileupdate.Input{Alias: "production", Patch: profileupdate.Patch{APIVersion: profileupdate.StringField{Set: true, Value: "3.29"}}})
	if err != nil || got.Status != "unchanged" {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestOutputGoldens(t *testing.T) {
	store := &updater{result: profileupdate.UpdateResult{Profile: fixture(), ChangedFields: []string{"site_content_url"}}}
	got, _ := profileupdate.New(store).Execute(context.Background(), profileupdate.Input{Alias: "production", Patch: profileupdate.Patch{SiteContentURL: profileupdate.StringField{Set: true, Value: "marketing"}}})
	assertGolden(t, got, false, "testdata/output.toon")
	assertGolden(t, got, true, "testdata/output_full.toon")
}

func fixture() profileupdate.Profile {
	return profileupdate.Profile{Alias: "production", Default: true, ServerURL: "https://example.test", SiteContentURL: "marketing", APIVersion: "3.29", AuthType: "pat", PATNameEnv: "PROD_PAT_NAME", PATSecretEnv: "PROD_PAT_SECRET", DefaultWorkspace: "primary"}
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
