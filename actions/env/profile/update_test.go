package profile_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	profileupdate "github.com/ahillspace/tadx/actions/env/profile"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type updateTestStore struct {
	alias  string
	patch  profileupdate.Patch
	result profileupdate.UpdateResult
	err    error
}

func (u *updateTestStore) Update(_ context.Context, alias string, patch profileupdate.Patch) (profileupdate.UpdateResult, error) {
	u.alias, u.patch = alias, patch
	return u.result, u.err
}

func (u *updateTestStore) PreviewUpdate(ctx context.Context, alias string, patch profileupdate.Patch) (profileupdate.UpdateResult, error) {
	return u.Update(ctx, alias, patch)
}

type retryableStoreError struct{}

func TestUpdateCacheConcurrencyValidatedAndPassedToStore(t *testing.T) {
	for _, limit := range []int{-1, 0, 1, 256, 257} {
		store := &updateTestStore{}
		_, err := profileupdate.NewUpdate(store).Execute(context.Background(), profileupdate.UpdateInput{Alias: "staging", Patch: profileupdate.Patch{CacheMaxConcurrency: profileupdate.IntField{Set: true, Value: limit}}})
		if limit < 0 || limit > 256 {
			if err == nil || store.alias != "" {
				t.Fatalf("invalid%d error=%v stored=%+v", limit, err, store.patch)
			}
		} else if err != nil || !store.patch.CacheMaxConcurrency.Set || store.patch.CacheMaxConcurrency.Value != limit {
			t.Fatalf("limit%d error=%v stored=%+v", limit, err, store.patch)
		}
	}
}

func (retryableStoreError) Error() string   { return "candidate rejected" }
func (retryableStoreError) Retryable() bool { return true }
func (retryableStoreError) CorrectiveAction() string {
	return "Correct the complete candidate profile."
}

func TestUpdateExecuteRequiresAliasAndExplicitFields(t *testing.T) {
	store := &updateTestStore{}
	for _, input := range []profileupdate.UpdateInput{{}, {Alias: "production"}} {
		_, err := profileupdate.NewUpdate(store).Execute(context.Background(), input)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
			t.Fatalf("input %#v error = %#v", input, err)
		}
	}
}

func TestUpdateExecuteRejectsInvalidExplicitServerURLBeforeUpdater(t *testing.T) {
	store := &updateTestStore{}
	_, err := profileupdate.NewUpdate(store).Execute(context.Background(), profileupdate.UpdateInput{Alias: "Prod-West", Patch: profileupdate.Patch{ServerURL: profileupdate.StringField{Set: true, Value: "http://example.test"}}})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error = %#v", err)
	}
	if store.alias != "" {
		t.Fatalf("updater called for invalid URL with alias %q", store.alias)
	}
}

func TestUpdateExecutePreservesExactAliasAndStructuredUpdaterAdvice(t *testing.T) {
	store := &updateTestStore{err: retryableStoreError{}}
	_, err := profileupdate.NewUpdate(store).Execute(context.Background(), profileupdate.UpdateInput{Alias: "Prod-West", Patch: profileupdate.Patch{PATNameEnv: profileupdate.StringField{Set: true, Value: "NEW_NAME_REF"}}})
	payload := errs.Structure(err).Error
	if store.alias != "Prod-West" || payload.ID != "env.profile.update.write" || payload.Retryable == nil || !*payload.Retryable || payload.CorrectiveAction != "Correct the complete candidate profile." {
		t.Fatalf("alias = %q, error = %#v", store.alias, payload)
	}
}

func TestUpdateExecutePreservesStoreChangedFields(t *testing.T) {
	result := profileupdate.UpdateResult{Profile: updateFixture(), ChangedFields: []string{"site_content_url", "api_version"}}
	store := &updateTestStore{result: result}
	patch := profileupdate.Patch{SiteContentURL: profileupdate.StringField{Set: true, Value: "marketing"}, APIVersion: profileupdate.StringField{Set: true, Value: "3.29"}}
	got, err := profileupdate.NewUpdate(store).Execute(context.Background(), profileupdate.UpdateInput{Alias: "production", Patch: patch})
	if err != nil || got.Status != "updated" || !reflect.DeepEqual(got.ChangedFields, []string{"site_content_url", "api_version"}) {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestUpdateExecuteReturnsUnchanged(t *testing.T) {
	store := &updateTestStore{result: profileupdate.UpdateResult{Profile: updateFixture()}}
	got, err := profileupdate.NewUpdate(store).Execute(context.Background(), profileupdate.UpdateInput{Alias: "production", Patch: profileupdate.Patch{APIVersion: profileupdate.StringField{Set: true, Value: "3.29"}}})
	if err != nil || got.Status != "unchanged" {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestUpdateOutputGoldens(t *testing.T) {
	store := &updateTestStore{result: profileupdate.UpdateResult{Profile: updateFixture(), ChangedFields: []string{"site_content_url"}}}
	got, _ := profileupdate.NewUpdate(store).Execute(context.Background(), profileupdate.UpdateInput{Alias: "production", Patch: profileupdate.Patch{SiteContentURL: profileupdate.StringField{Set: true, Value: "marketing"}}})
	updateAssertGolden(t, got, false, "testdata/update/output.toon")
	updateAssertGolden(t, got, true, "testdata/update/output_full.toon")
}

func updateFixture() profileupdate.UpdateProfile {
	return profileupdate.UpdateProfile{Alias: "production", Default: true, ServerURL: "https://example.test", SiteContentURL: "marketing", APIVersion: "3.29", AuthType: "pat", PATNameEnv: "PROD_PAT_NAME", PATSecretEnv: "PROD_PAT_SECRET", DefaultWorkspace: "primary"}
}
func updateAssertGolden(t *testing.T, value any, full bool, path string) {
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
