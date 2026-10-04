package env_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	profilelist "github.com/ahillspace/tadx/actions/env"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type listTestReader struct {
	profilelist.Store
	profiles []profilelist.Profile
	err      error
}

type listDirectReader struct {
	profilelist.Store
	profiles []profilelist.Profile
}

func (r listDirectReader) List(context.Context) ([]profilelist.Profile, error) {
	return r.profiles, nil
}

func (r listTestReader) List(context.Context) ([]profilelist.Profile, error) {
	return append([]profilelist.Profile(nil), r.profiles...), r.err
}

func TestServiceListSortsAndPagesProfiles(t *testing.T) {
	source := []profilelist.Profile{{Alias: "zeta"}, {Alias: "Prod-West", Default: true}}
	action := profilelist.New(listDirectReader{profiles: source})
	got, err := action.List(context.Background(), profilelist.ListInput{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got.Page != (profilelist.Page{Returned: 1, Total: 2, Limit: 1, NextCursor: "1"}) || got.Profiles[0].Alias != "Prod-West" {
		t.Fatalf("output = %#v", got)
	}
	if source[0].Alias != "zeta" {
		t.Fatalf("source was mutated: %#v", source)
	}
}

func TestServiceListAllReturnsCompleteProfileInventory(t *testing.T) {
	profiles := []profilelist.Profile{{Alias: "alpha"}, {Alias: "beta"}}
	got, err := profilelist.New(listDirectReader{profiles: profiles}).List(context.Background(), profilelist.ListInput{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Page.Returned != 2 || got.Page.Total != 2 || got.Page.Limit != profilelist.ListMaxLimit || got.Page.NextCursor != "" || len(got.Profiles) != 2 {
		t.Fatalf("all page = %#v", got.Page)
	}
}

func TestServiceListAllRejectsPaginationOverridesAndOverflow(t *testing.T) {
	for _, input := range []profilelist.ListInput{{All: true, Limit: 1}, {All: true, Cursor: "0"}} {
		if _, err := profilelist.New(listDirectReader{}).List(context.Background(), input); err == nil {
			t.Fatalf("Execute(%#v) error = nil", input)
		}
	}
	profiles := make([]profilelist.Profile, profilelist.ListMaxLimit+1)
	_, err := profilelist.New(listDirectReader{profiles: profiles}).List(context.Background(), profilelist.ListInput{All: true})
	if err == nil {
		t.Fatal("overflow Execute() error = nil")
	}
}

func TestServiceListContinuationTerminalAndEmptyPages(t *testing.T) {
	action := profilelist.New(listTestReader{profiles: []profilelist.Profile{{Alias: "alpha"}, {Alias: "beta"}}})
	terminal, err := action.List(context.Background(), profilelist.ListInput{Limit: 1, Cursor: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Page.NextCursor != "" || terminal.Page.Returned != 1 || len(terminal.Help) != 1 {
		t.Fatalf("terminal page = %#v", terminal)
	}
	empty, err := profilelist.New(listTestReader{}).List(context.Background(), profilelist.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Page.Total != 0 || empty.Page.Returned != 0 || empty.Profiles == nil {
		t.Fatalf("empty page = %#v", empty)
	}
}

func TestServiceListAcceptsMaxLimitAndRejectsPastEndCursor(t *testing.T) {
	got, err := profilelist.New(listTestReader{}).List(context.Background(), profilelist.ListInput{Limit: profilelist.ListMaxLimit})
	if err != nil || got.Page.Limit != profilelist.ListMaxLimit {
		t.Fatalf("max limit output = %#v, error = %v", got, err)
	}
	_, err = profilelist.New(listTestReader{}).List(context.Background(), profilelist.ListInput{Cursor: "1"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("past-end error = %#v", err)
	}
}

func TestServiceListValidatesBoundsAndWrapsReadFailure(t *testing.T) {
	for _, input := range []profilelist.ListInput{{Limit: -1}, {Limit: 10001}, {Cursor: "bad"}} {
		_, err := profilelist.New(listTestReader{}).List(context.Background(), input)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
			t.Fatalf("input %#v error = %#v", input, err)
		}
	}
	_, err := profilelist.New(listTestReader{err: errors.New("read failed")}).List(context.Background(), profilelist.ListInput{})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "env.profile.list.read" {
		t.Fatalf("read error = %#v", err)
	}
}

func TestListOutputProjectionsAndGoldens(t *testing.T) {
	value, err := profilelist.New(listTestReader{profiles: []profilelist.Profile{{Alias: "production", Default: true, ServerURL: "https://example.test", SiteContentURL: "marketing", APIVersion: "3.29", AuthType: "pat", PATNameEnv: "PROD_PAT_NAME", PATSecretEnv: "PROD_PAT_SECRET", DefaultWorkspace: "primary"}}}).List(context.Background(), profilelist.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	listAssertGolden(t, value, false, "testdata/list/output.toon")
	listAssertGolden(t, value, true, "testdata/list/output_full.toon")
	compact := value.CompactOutput().(profilelist.ListCompactResult)
	full := value.FullOutput().(profilelist.ListOutput)
	if compact.Details != "--full" || full.Profiles[0].PATSecretEnv != "PROD_PAT_SECRET" || compact.Profiles[0].Alias != "production" {
		t.Fatalf("compact = %#v, full = %#v", compact, full)
	}
	if reflect.DeepEqual(compact, full) {
		t.Fatal("compact and full projections are equal")
	}
}

func listAssertGolden(t *testing.T, value any, full bool, path string) {
	t.Helper()
	var actual bytes.Buffer
	if err := output.RenderWithOptions(&actual, value, output.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual.Bytes(), want) {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, actual.Bytes())
	}
}
