package profile_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/actions/env/profile"
	"github.com/ahillspace/tadx/internal/errs"
)

// A reference that is not a variable name is most likely a pasted secret, so
// rejection must happen before any store call and must never echo the value.
const secretShapedReference = "abc123DEF==:ghiJKL456"

func TestAddRejectsInvalidPATVariableReferencesWithoutEchoing(t *testing.T) {
	for _, input := range []profile.AddInput{
		{Alias: "probe", ServerURL: "https://example.test", PATSecretEnv: secretShapedReference},
		{Alias: "probe", ServerURL: "https://example.test", PATNameEnv: secretShapedReference},
		{Alias: "probe", ServerURL: "https://example.test", PATNameEnv: "VALID_NAME", PATSecretEnv: secretShapedReference, Preview: true},
	} {
		store := &addTestStore{}
		_, err := profile.NewAdd(store).Execute(context.Background(), input)
		assertRejectedWithoutEcho(t, err, "env.profile.add.usage")
		if store.got.Alias != "" {
			t.Fatalf("store called with %#v", store.got)
		}
	}
}

func TestAddAcceptsValidPATVariableReferences(t *testing.T) {
	store := &addTestStore{result: addFixture()}
	_, err := profile.NewAdd(store).Execute(context.Background(), profile.AddInput{Alias: "probe", ServerURL: "https://example.test", PATNameEnv: "PROBE_PAT_NAME", PATSecretEnv: "PROBE_PAT_SECRET"})
	if err != nil || store.got.PATNameEnv != "PROBE_PAT_NAME" || store.got.PATSecretEnv != "PROBE_PAT_SECRET" {
		t.Fatalf("stored = %#v, error = %v", store.got, err)
	}
}

func TestUpdateRejectsInvalidPATVariableReferencesWithoutEchoing(t *testing.T) {
	for _, patch := range []profile.Patch{
		{PATSecretEnv: profile.StringField{Set: true, Value: secretShapedReference}},
		{PATNameEnv: profile.StringField{Set: true, Value: secretShapedReference}},
		{PATNameEnv: profile.StringField{Set: true, Value: "VALID_NAME"}, PATSecretEnv: profile.StringField{Set: true, Value: "has space"}},
	} {
		for _, preview := range []bool{false, true} {
			store := &updateTestStore{}
			_, err := profile.NewUpdate(store).Execute(context.Background(), profile.UpdateInput{Alias: "probe", Preview: preview, Patch: patch})
			assertRejectedWithoutEcho(t, err, "env.profile.update.usage")
			if store.alias != "" {
				t.Fatalf("updater called with %#v", store.patch)
			}
		}
	}
}

func TestUpdateAcceptsClearedAndValidPATVariableReferences(t *testing.T) {
	store := &updateTestStore{}
	patch := profile.Patch{PATNameEnv: profile.StringField{Set: true, Value: ""}, PATSecretEnv: profile.StringField{Set: true, Value: "NEW_PAT_SECRET"}}
	if _, err := profile.NewUpdate(store).Execute(context.Background(), profile.UpdateInput{Alias: "probe", Patch: patch}); err != nil {
		t.Fatalf("error = %v", err)
	}
	if store.patch != patch {
		t.Fatalf("patch = %#v", store.patch)
	}
}

func assertRejectedWithoutEcho(t *testing.T, err error, id string) {
	t.Helper()
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.ID != id {
		t.Fatalf("error = %#v", err)
	}
	encoded, marshalErr := json.Marshal(errs.Structure(err))
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	for _, text := range []string{err.Error(), string(encoded)} {
		if strings.Contains(text, secretShapedReference) || strings.Contains(text, "has space") {
			t.Fatalf("rejected reference echoed: %s", text)
		}
	}
}
