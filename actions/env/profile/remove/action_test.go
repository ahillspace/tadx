package remove_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	profileremove "github.com/ahillspace/tadx/actions/env/profile/remove"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type remover struct {
	alias string
	err   error
}

func TestOutputGolden(t *testing.T) {
	got, err := profileremove.New(&remover{}).Execute(context.Background(), profileremove.Input{Alias: "production"})
	if err != nil {
		t.Fatal(err)
	}
	var actual bytes.Buffer
	if err := output.Render(&actual, got); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/output.toon")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual.Bytes(), want) {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, actual.Bytes())
	}
}

func (r *remover) Remove(_ context.Context, alias string) error { r.alias = alias; return r.err }

func TestExecuteRequiresAliasAndReturnsRemovedStatus(t *testing.T) {
	store := &remover{}
	_, err := profileremove.New(store).Execute(context.Background(), profileremove.Input{})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error = %#v", err)
	}
	got, err := profileremove.New(store).Execute(context.Background(), profileremove.Input{Alias: "Prod-West"})
	if err != nil || got.Status != "removed" || got.Environment != "Prod-West" || store.alias != "Prod-West" {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}
