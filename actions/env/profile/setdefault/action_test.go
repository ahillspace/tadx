package setdefault_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	setdefault "github.com/ahillspace/tadx/actions/env/profile/setdefault"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type setter struct {
	alias   string
	changed bool
	err     error
}

func TestOutputGolden(t *testing.T) {
	got, err := setdefault.New(&setter{changed: true}).Execute(context.Background(), setdefault.Input{Alias: "production"})
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

func (s *setter) SetDefault(_ context.Context, alias string) (bool, error) {
	s.alias = alias
	return s.changed, s.err
}

func TestExecuteRequiresAliasAndReportsChangeState(t *testing.T) {
	store := &setter{changed: true}
	_, err := setdefault.New(store).Execute(context.Background(), setdefault.Input{})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error = %#v", err)
	}
	got, err := setdefault.New(store).Execute(context.Background(), setdefault.Input{Alias: "Prod-West"})
	if err != nil || got.Status != "updated" || got.DefaultEnvironment != "Prod-West" || store.alias != "Prod-West" {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
	store.changed = false
	got, _ = setdefault.New(store).Execute(context.Background(), setdefault.Input{Alias: "production"})
	if got.Status != "unchanged" {
		t.Fatalf("output = %#v", got)
	}
}
