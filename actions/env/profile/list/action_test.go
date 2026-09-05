package list_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	profilelist "github.com/ahillspace/tadx/actions/env/profile/list"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type reader struct {
	profiles []profilelist.Profile
	err      error
}

type directReader struct{ profiles []profilelist.Profile }

func (r directReader) List(context.Context) ([]profilelist.Profile, error) { return r.profiles, nil }

func (r reader) List(context.Context) ([]profilelist.Profile, error) {
	return append([]profilelist.Profile(nil), r.profiles...), r.err
}

func TestExecuteSortsAndPagesProfiles(t *testing.T) {
	source := []profilelist.Profile{{Alias: "zeta"}, {Alias: "Prod-West", Default: true}}
	action := profilelist.New(directReader{profiles: source})
	got, err := action.Execute(context.Background(), profilelist.Input{Limit: 1})
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

func TestExecuteContinuationTerminalAndEmptyPages(t *testing.T) {
	action := profilelist.New(reader{profiles: []profilelist.Profile{{Alias: "alpha"}, {Alias: "beta"}}})
	terminal, err := action.Execute(context.Background(), profilelist.Input{Limit: 1, Cursor: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Page.NextCursor != "" || terminal.Page.Returned != 1 || len(terminal.Help) != 1 {
		t.Fatalf("terminal page = %#v", terminal)
	}
	empty, err := profilelist.New(reader{}).Execute(context.Background(), profilelist.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Page.Total != 0 || empty.Page.Returned != 0 || empty.Profiles == nil {
		t.Fatalf("empty page = %#v", empty)
	}
}

func TestExecuteAcceptsMaxLimitAndRejectsPastEndCursor(t *testing.T) {
	got, err := profilelist.New(reader{}).Execute(context.Background(), profilelist.Input{Limit: profilelist.MaxLimit})
	if err != nil || got.Page.Limit != profilelist.MaxLimit {
		t.Fatalf("max limit output = %#v, error = %v", got, err)
	}
	_, err = profilelist.New(reader{}).Execute(context.Background(), profilelist.Input{Cursor: "1"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("past-end error = %#v", err)
	}
}

func TestExecuteValidatesBoundsAndWrapsReadFailure(t *testing.T) {
	for _, input := range []profilelist.Input{{Limit: -1}, {Limit: 101}, {Cursor: "bad"}} {
		_, err := profilelist.New(reader{}).Execute(context.Background(), input)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
			t.Fatalf("input %#v error = %#v", input, err)
		}
	}
	_, err := profilelist.New(reader{err: errors.New("read failed")}).Execute(context.Background(), profilelist.Input{})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "env.profile.list.read" {
		t.Fatalf("read error = %#v", err)
	}
}

func TestOutputProjectionsAndGoldens(t *testing.T) {
	value, err := profilelist.New(reader{profiles: []profilelist.Profile{{Alias: "production", Default: true, ServerURL: "https://example.test", SiteContentURL: "marketing", APIVersion: "3.29", AuthType: "pat", PATNameEnv: "PROD_PAT_NAME", PATSecretEnv: "PROD_PAT_SECRET", DefaultWorkspace: "primary"}}}).Execute(context.Background(), profilelist.Input{})
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, value, false, "testdata/output.toon")
	assertGolden(t, value, true, "testdata/output_full.toon")
	compact := value.CompactOutput().(profilelist.CompactResult)
	full := value.FullOutput().(profilelist.FullResult)
	if compact.Details != "--full" || full.Profiles[0].PATSecretEnv != "PROD_PAT_SECRET" || compact.Profiles[0].Alias != "production" {
		t.Fatalf("compact = %#v, full = %#v", compact, full)
	}
	if reflect.DeepEqual(compact, full) {
		t.Fatal("compact and full projections are equal")
	}
}

func assertGolden(t *testing.T, value any, full bool, path string) {
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
