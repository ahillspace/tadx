package env_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	setdefault "github.com/ahillspace/tadx/actions/env"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type defaultTestStore struct {
	setdefault.Store
	alias   string
	changed bool
	err     error
}

func TestSetDefaultOutputGolden(t *testing.T) {
	got, err := setdefault.New(&defaultTestStore{changed: true}).SetDefault(context.Background(), setdefault.SetDefaultInput{Alias: "production"})
	if err != nil {
		t.Fatal(err)
	}
	var actual bytes.Buffer
	if err := output.Render(&actual, got); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/setdefault/output.toon")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual.Bytes(), want) {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, actual.Bytes())
	}
}

func (s *defaultTestStore) SetDefault(_ context.Context, alias string) (bool, error) {
	s.alias = alias
	return s.changed, s.err
}

func (s *defaultTestStore) PreviewSetDefault(ctx context.Context, alias string) (bool, error) {
	return s.SetDefault(ctx, alias)
}

func TestServiceSetDefaultRequiresAliasAndReportsChangeState(t *testing.T) {
	store := &defaultTestStore{changed: true}
	_, err := setdefault.New(store).SetDefault(context.Background(), setdefault.SetDefaultInput{})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error = %#v", err)
	}
	got, err := setdefault.New(store).SetDefault(context.Background(), setdefault.SetDefaultInput{Alias: "Prod-West"})
	if err != nil || got.Status != "updated" || got.DefaultEnvironment != "Prod-West" || store.alias != "Prod-West" {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
	store.changed = false
	got, _ = setdefault.New(store).SetDefault(context.Background(), setdefault.SetDefaultInput{Alias: "production"})
	if got.Status != "unchanged" {
		t.Fatalf("output = %#v", got)
	}
}
