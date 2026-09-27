package profile_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	profileremove "github.com/ahillspace/tadx/actions/env/profile"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type removeTestStore struct {
	alias string
	err   error
}

func TestRemoveOutputGolden(t *testing.T) {
	got, err := profileremove.NewRemove(&removeTestStore{}).Execute(context.Background(), profileremove.RemoveInput{Alias: "production"})
	if err != nil {
		t.Fatal(err)
	}
	var actual bytes.Buffer
	if err := output.Render(&actual, got); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/remove/output.toon")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual.Bytes(), want) {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, actual.Bytes())
	}
}

func (r *removeTestStore) Remove(_ context.Context, alias string) error {
	r.alias = alias
	return r.err
}

func (r *removeTestStore) PreviewRemove(ctx context.Context, alias string) error {
	return r.Remove(ctx, alias)
}

func TestRemoveExecuteRequiresAliasAndReturnsRemovedStatus(t *testing.T) {
	store := &removeTestStore{}
	_, err := profileremove.NewRemove(store).Execute(context.Background(), profileremove.RemoveInput{})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error = %#v", err)
	}
	got, err := profileremove.NewRemove(store).Execute(context.Background(), profileremove.RemoveInput{Alias: "Prod-West"})
	if err != nil || got.Status != "removed" || got.Environment != "Prod-West" || store.alias != "Prod-West" {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}
